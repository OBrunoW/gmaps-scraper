package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeQueue struct {
	enqueueID    int64
	enqueueErr   error
	enqueued     []domain.ScrapeParams
	records      map[int64]domain.JobRecord
	getErr       error
	page         domain.JobPage
	listErr      error
	listFilters  []domain.JobListFilter
	cancelErr    error
	cancelled    []int64
	purgeErr     error
	purgeQueued  []int64
	getCallCount int
}

func (q *fakeQueue) Enqueue(_ context.Context, params domain.ScrapeParams) (int64, error) {
	q.enqueued = append(q.enqueued, params)
	return q.enqueueID, q.enqueueErr
}

func (q *fakeQueue) Get(_ context.Context, id int64) (domain.JobRecord, error) {
	q.getCallCount++

	if q.getErr != nil {
		return domain.JobRecord{}, q.getErr
	}

	record, ok := q.records[id]
	if !ok {
		return domain.JobRecord{}, errors.New("no rows")
	}

	return record, nil
}

func (q *fakeQueue) List(_ context.Context, filter domain.JobListFilter) (domain.JobPage, error) {
	q.listFilters = append(q.listFilters, filter)
	return q.page, q.listErr
}

func (q *fakeQueue) Cancel(_ context.Context, id int64) error {
	q.cancelled = append(q.cancelled, id)
	return q.cancelErr
}

func (q *fakeQueue) EnqueuePurge(_ context.Context, id int64) error {
	q.purgeQueued = append(q.purgeQueued, id)
	return q.purgeErr
}

type fakeResults struct {
	found        domain.ScrapeResult
	findErr      error
	counts       map[int64]int
	countErr     error
	countCalls   [][]int64
	deleteRows   int64
	deleteErr    error
	deletedIDs   []int64
	expiredRows  int64
	expiredErr   error
	expiredCalls int
}

func (r *fakeResults) Save(context.Context, domain.ScrapeResult) error { return nil }

func (r *fakeResults) FindByJobID(context.Context, int64) (domain.ScrapeResult, error) {
	return r.found, r.findErr
}

func (r *fakeResults) CountByJobIDs(_ context.Context, ids []int64) (map[int64]int, error) {
	r.countCalls = append(r.countCalls, ids)
	return r.counts, r.countErr
}

func (r *fakeResults) DeleteByJobID(_ context.Context, id int64) (int64, error) {
	r.deletedIDs = append(r.deletedIDs, id)
	return r.deleteRows, r.deleteErr
}

func (r *fakeResults) DeleteExpired(context.Context) (int64, error) {
	r.expiredCalls++
	return r.expiredRows, r.expiredErr
}

type fakeJobs struct {
	err        error
	deletedIDs []int64
}

func (j *fakeJobs) Delete(_ context.Context, id int64) (int64, error) {
	j.deletedIDs = append(j.deletedIDs, id)
	return 1, j.err
}

type fakeCodec struct{}

func (fakeCodec) Encode(id int64) string { return "pub-" + strconv.FormatInt(id, 10) }

func (fakeCodec) Decode(publicID string) (int64, error) {
	if len(publicID) < 5 || publicID[:4] != "pub-" {
		return 0, errors.New("invalid job id")
	}

	return strconv.ParseInt(publicID[4:], 10, 64)
}

type serviceFixture struct {
	service *JobService
	queue   *fakeQueue
	results *fakeResults
	jobs    *fakeJobs
}

func newServiceFixture() serviceFixture {
	queue := &fakeQueue{records: map[int64]domain.JobRecord{}}
	results := &fakeResults{}
	jobs := &fakeJobs{}

	return serviceFixture{
		service: NewJobService(queue, results, jobs, fakeCodec{}),
		queue:   queue,
		results: results,
		jobs:    jobs,
	}
}

func requireAppError(t *testing.T, err error, status int, code, message string) {
	t.Helper()

	var appErr *domain.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, status, appErr.Status)
	require.Equal(t, code, appErr.Code)
	require.Equal(t, message, appErr.Message)
}

func TestCreateReturnsPendingJobWithEncodedID(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.enqueueID = 42

	created, err := fixture.service.Create(context.Background(), domain.ScrapeParams{Keyword: "pizza"})
	require.NoError(t, err)
	require.Equal(t, "pub-42", created.PublicID)
	require.Equal(t, domain.StatusPending, created.Status)
	require.Len(t, fixture.queue.enqueued, 1)
	require.Equal(t, "en", fixture.queue.enqueued[0].Lang)
	require.Equal(t, 1, fixture.queue.enqueued[0].MaxDepth)
	require.Equal(t, domain.DefaultScrapeTimeoutSecs, fixture.queue.enqueued[0].TimeoutSecs)
}

func TestCreateRejectsInvalidParams(t *testing.T) {
	fixture := newServiceFixture()

	_, err := fixture.service.Create(context.Background(), domain.ScrapeParams{})
	requireAppError(t, err, http.StatusBadRequest, domain.CodeInvalidInput, "keyword is required")
	require.Empty(t, fixture.queue.enqueued)
}

func TestCreateMapsEnqueueFailure(t *testing.T) {
	fixture := newServiceFixture()
	cause := errors.New("db down")
	fixture.queue.enqueueErr = cause

	_, err := fixture.service.Create(context.Background(), domain.ScrapeParams{Keyword: "pizza"})
	requireAppError(t, err, http.StatusInternalServerError, domain.CodeInternal, "failed to create job")
	require.ErrorIs(t, err, cause)
}

func TestListRejectsUnknownState(t *testing.T) {
	fixture := newServiceFixture()

	_, err := fixture.service.List(context.Background(), ListJobsInput{State: "bogus"})
	requireAppError(t, err, http.StatusBadRequest, domain.CodeInvalidInput, "invalid state: bogus")
	require.Empty(t, fixture.queue.listFilters)
}

func TestListPassesStateAndCursor(t *testing.T) {
	fixture := newServiceFixture()

	_, err := fixture.service.List(context.Background(), ListJobsInput{State: "running", Limit: 5, Cursor: "abc"})
	require.NoError(t, err)
	require.Equal(t, []domain.JobListFilter{{State: domain.StateRunning, Limit: 5, Cursor: "abc"}}, fixture.queue.listFilters)
}

func TestListClampsLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{"zero", 0, 20},
		{"negative", -3, 20},
		{"within", 50, 50},
		{"max", 100, 100},
		{"above max", 101, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newServiceFixture()

			_, err := fixture.service.List(context.Background(), ListJobsInput{Limit: tt.limit})
			require.NoError(t, err)
			require.Equal(t, tt.want, fixture.queue.listFilters[0].Limit)
		})
	}
}

func TestListQueueErrorBecomesBadRequestWithRawText(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.listErr = errors.New("invalid cursor: bad base64")

	_, err := fixture.service.List(context.Background(), ListJobsInput{Cursor: "x"})
	requireAppError(t, err, http.StatusBadRequest, domain.CodeInvalidInput, "invalid cursor: bad base64")
}

func TestListKeepsAppErrorFromQueue(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.listErr = domain.Internal("boom", nil)

	_, err := fixture.service.List(context.Background(), ListJobsInput{})
	requireAppError(t, err, http.StatusInternalServerError, domain.CodeInternal, "boom")
}

func TestListHasMoreWhenPageIsFull(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.page = domain.JobPage{
		Records:    []domain.JobRecord{{ID: 1, State: domain.StateAvailable}, {ID: 2, State: domain.StateAvailable}},
		NextCursor: "next",
	}

	list, err := fixture.service.List(context.Background(), ListJobsInput{Limit: 2})
	require.NoError(t, err)
	require.True(t, list.HasMore)
	require.Equal(t, "next", list.NextCursor)

	list, err = fixture.service.List(context.Background(), ListJobsInput{Limit: 3})
	require.NoError(t, err)
	require.False(t, list.HasMore)
}

func TestListEmptyPageReturnsNonNilJobs(t *testing.T) {
	fixture := newServiceFixture()

	list, err := fixture.service.List(context.Background(), ListJobsInput{})
	require.NoError(t, err)
	require.NotNil(t, list.Jobs)
	require.Empty(t, list.Jobs)
	require.False(t, list.HasMore)
}

func TestListMapsEveryState(t *testing.T) {
	startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	createdAt := startedAt.Add(-time.Minute)

	fixture := newServiceFixture()
	fixture.results.counts = map[int64]int{5: 25}
	fixture.queue.page = domain.JobPage{Records: []domain.JobRecord{
		{ID: 1, State: domain.StateAvailable, Keyword: "a", CreatedAt: createdAt},
		{ID: 2, State: domain.StateScheduled, Keyword: "b", CreatedAt: createdAt},
		{ID: 3, State: domain.StateRetryable, Keyword: "c", CreatedAt: createdAt},
		{ID: 4, State: domain.StatePending, Keyword: "d", CreatedAt: createdAt},
		{ID: 5, State: domain.StateCompleted, Keyword: "e", CreatedAt: createdAt, AttemptedAt: &startedAt, FinalizedAt: &finishedAt},
		{ID: 6, State: domain.StateRunning, Keyword: "f", CreatedAt: createdAt, AttemptedAt: &startedAt},
		{ID: 7, State: domain.StateCancelled, Keyword: "g", CreatedAt: createdAt, LastError: "cancelled by user"},
		{ID: 8, State: domain.StateDiscarded, Keyword: "h", CreatedAt: createdAt, LastError: "max attempts"},
	}}

	list, err := fixture.service.List(context.Background(), ListJobsInput{Limit: 100})
	require.NoError(t, err)
	require.Len(t, list.Jobs, 8)

	for index := 0; index < 4; index++ {
		require.Equal(t, domain.StatusPending, list.Jobs[index].Status)
		require.Nil(t, list.Jobs[index].StartedAt)
		require.Nil(t, list.Jobs[index].CompletedAt)
	}

	require.Equal(t, "pub-1", list.Jobs[0].PublicID)
	require.Equal(t, "a", list.Jobs[0].Keyword)
	require.Equal(t, createdAt, list.Jobs[0].CreatedAt)

	completed := list.Jobs[4]
	require.Equal(t, domain.StatusCompleted, completed.Status)
	require.Equal(t, &startedAt, completed.StartedAt)
	require.Equal(t, &finishedAt, completed.CompletedAt)
	require.Equal(t, 25, completed.ResultCount)
	require.Nil(t, completed.Results)

	running := list.Jobs[5]
	require.Equal(t, domain.StatusRunning, running.Status)
	require.Equal(t, &startedAt, running.StartedAt)
	require.Nil(t, running.CompletedAt)

	require.Equal(t, domain.StatusFailed, list.Jobs[6].Status)
	require.Equal(t, "cancelled by user", list.Jobs[6].Error)
	require.Equal(t, domain.StatusFailed, list.Jobs[7].Status)
	require.Equal(t, "max attempts", list.Jobs[7].Error)
}

func TestListCountsOnlyCompletedJobs(t *testing.T) {
	fixture := newServiceFixture()
	fixture.results.counts = map[int64]int{2: 7}
	fixture.queue.page = domain.JobPage{Records: []domain.JobRecord{
		{ID: 1, State: domain.StateRunning},
		{ID: 2, State: domain.StateCompleted},
		{ID: 3, State: domain.StateDiscarded},
	}}

	_, err := fixture.service.List(context.Background(), ListJobsInput{})
	require.NoError(t, err)
	require.Equal(t, [][]int64{{2}}, fixture.results.countCalls)
}

func TestListSkipsCountWithoutCompletedJobs(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.page = domain.JobPage{Records: []domain.JobRecord{{ID: 1, State: domain.StateRunning}}}

	_, err := fixture.service.List(context.Background(), ListJobsInput{})
	require.NoError(t, err)
	require.Empty(t, fixture.results.countCalls)
}

func TestListCountErrorFallsBackToZeroCounts(t *testing.T) {
	fixture := newServiceFixture()
	fixture.results.countErr = errors.New("db down")
	fixture.results.counts = map[int64]int{1: 9}
	fixture.queue.page = domain.JobPage{Records: []domain.JobRecord{{ID: 1, State: domain.StateCompleted}}}

	list, err := fixture.service.List(context.Background(), ListJobsInput{})
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, list.Jobs[0].Status)
	require.Equal(t, 0, list.Jobs[0].ResultCount)
}

func TestGetUnknownPublicIDIsNotFound(t *testing.T) {
	fixture := newServiceFixture()

	_, err := fixture.service.Get(context.Background(), "garbage")
	requireAppError(t, err, http.StatusNotFound, domain.CodeNotFound, "job not found")
	require.Zero(t, fixture.queue.getCallCount)
}

func TestGetQueueErrorIsNotFound(t *testing.T) {
	fixture := newServiceFixture()

	_, err := fixture.service.Get(context.Background(), "pub-9")
	requireAppError(t, err, http.StatusNotFound, domain.CodeNotFound, "job not found")
}

func TestGetMapsEveryState(t *testing.T) {
	startedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	createdAt := startedAt.Add(-time.Minute)
	results := json.RawMessage(`[{"title":"x"}]`)

	tests := []struct {
		name          string
		record        domain.JobRecord
		wantStatus    domain.JobStatus
		wantStarted   *time.Time
		wantCompleted *time.Time
		wantResults   json.RawMessage
		wantCount     int
		wantError     string
	}{
		{"available", domain.JobRecord{State: domain.StateAvailable}, domain.StatusPending, nil, nil, nil, 0, ""},
		{"scheduled", domain.JobRecord{State: domain.StateScheduled}, domain.StatusPending, nil, nil, nil, 0, ""},
		{"retryable", domain.JobRecord{State: domain.StateRetryable}, domain.StatusPending, nil, nil, nil, 0, ""},
		{"pending", domain.JobRecord{State: domain.StatePending}, domain.StatusPending, nil, nil, nil, 0, ""},
		{"running", domain.JobRecord{State: domain.StateRunning, AttemptedAt: &startedAt}, domain.StatusRunning, &startedAt, nil, nil, 0, ""},
		{
			"completed",
			domain.JobRecord{State: domain.StateCompleted, AttemptedAt: &startedAt, FinalizedAt: &finishedAt},
			domain.StatusCompleted, &startedAt, &finishedAt, results, 1, "",
		},
		{"cancelled", domain.JobRecord{State: domain.StateCancelled, LastError: "stopped"}, domain.StatusFailed, nil, nil, nil, 0, "stopped"},
		{"discarded", domain.JobRecord{State: domain.StateDiscarded, LastError: "max attempts"}, domain.StatusFailed, nil, nil, nil, 0, "max attempts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newServiceFixture()
			tt.record.ID = 9
			tt.record.Keyword = "pizza"
			tt.record.CreatedAt = createdAt
			fixture.queue.records[9] = tt.record
			fixture.results.found = domain.ScrapeResult{JobID: 9, Results: results, ResultCount: 1}

			view, err := fixture.service.Get(context.Background(), "pub-9")
			require.NoError(t, err)
			require.Equal(t, "pub-9", view.PublicID)
			require.Equal(t, "pizza", view.Keyword)
			require.Equal(t, createdAt, view.CreatedAt)
			require.Equal(t, tt.wantStatus, view.Status)
			require.Equal(t, tt.wantStarted, view.StartedAt)
			require.Equal(t, tt.wantCompleted, view.CompletedAt)
			require.Equal(t, tt.wantResults, view.Results)
			require.Equal(t, tt.wantCount, view.ResultCount)
			require.Equal(t, tt.wantError, view.Error)
		})
	}
}

func TestGetPendingStateReturnsPendingStatus(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.records[3] = domain.JobRecord{ID: 3, State: domain.StatePending}

	view, err := fixture.service.Get(context.Background(), "pub-3")
	require.NoError(t, err)
	require.Equal(t, domain.StatusPending, view.Status)
}

func TestGetCompletedIgnoresResultReadError(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.records[4] = domain.JobRecord{ID: 4, State: domain.StateCompleted}
	fixture.results.findErr = errors.New("no rows")
	fixture.results.found = domain.ScrapeResult{Results: json.RawMessage(`[1]`), ResultCount: 5}

	view, err := fixture.service.Get(context.Background(), "pub-4")
	require.NoError(t, err)
	require.Equal(t, domain.StatusCompleted, view.Status)
	require.Nil(t, view.Results)
	require.Equal(t, 0, view.ResultCount)
}

func TestDeleteInvalidPublicIDIsInternalError(t *testing.T) {
	fixture := newServiceFixture()

	err := fixture.service.Delete(context.Background(), "garbage")
	requireAppError(t, err, http.StatusInternalServerError, domain.CodeInternal, "invalid job id: invalid job id")
	require.Empty(t, fixture.queue.purgeQueued)
}

func TestDeleteMissingJobIsNotFound(t *testing.T) {
	fixture := newServiceFixture()

	err := fixture.service.Delete(context.Background(), "pub-1")
	requireAppError(t, err, http.StatusNotFound, domain.CodeNotFound, "job not found")
	require.Empty(t, fixture.queue.purgeQueued)
}

func TestDeleteRunningJobCancelsThenQueuesPurge(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.records[1] = domain.JobRecord{ID: 1, State: domain.StateRunning}

	require.NoError(t, fixture.service.Delete(context.Background(), "pub-1"))
	require.Equal(t, []int64{1}, fixture.queue.cancelled)
	require.Equal(t, []int64{1}, fixture.queue.purgeQueued)
}

func TestDeleteIgnoresCancelError(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.records[1] = domain.JobRecord{ID: 1, State: domain.StateRunning}
	fixture.queue.cancelErr = errors.New("cancel failed")

	require.NoError(t, fixture.service.Delete(context.Background(), "pub-1"))
	require.Equal(t, []int64{1}, fixture.queue.purgeQueued)
}

func TestDeleteNonRunningJobDoesNotCancel(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.records[1] = domain.JobRecord{ID: 1, State: domain.StateCompleted}

	require.NoError(t, fixture.service.Delete(context.Background(), "pub-1"))
	require.Empty(t, fixture.queue.cancelled)
	require.Equal(t, []int64{1}, fixture.queue.purgeQueued)
}

func TestDeleteMapsPurgeEnqueueFailure(t *testing.T) {
	fixture := newServiceFixture()
	fixture.queue.records[1] = domain.JobRecord{ID: 1, State: domain.StateCompleted}
	fixture.queue.purgeErr = errors.New("insert failed")

	err := fixture.service.Delete(context.Background(), "pub-1")
	requireAppError(t, err, http.StatusInternalServerError, domain.CodeInternal, "failed to queue deletion: insert failed")
}

func TestPurgeDeletesResultsThenJob(t *testing.T) {
	fixture := newServiceFixture()

	require.NoError(t, fixture.service.Purge(context.Background(), 7))
	require.Equal(t, []int64{7}, fixture.results.deletedIDs)
	require.Equal(t, []int64{7}, fixture.jobs.deletedIDs)
}

func TestPurgeResultsFailureStopsBeforeJobDelete(t *testing.T) {
	fixture := newServiceFixture()
	cause := errors.New("db down")
	fixture.results.deleteErr = cause

	err := fixture.service.Purge(context.Background(), 7)
	require.EqualError(t, err, "failed to delete results: db down")
	require.ErrorIs(t, err, cause)
	require.Empty(t, fixture.jobs.deletedIDs)
}

func TestPurgeJobFailure(t *testing.T) {
	fixture := newServiceFixture()
	cause := errors.New("db down")
	fixture.jobs.err = cause

	err := fixture.service.Purge(context.Background(), 7)
	require.EqualError(t, err, "failed to delete job: db down")
	require.ErrorIs(t, err, cause)
}

func TestCleanupExpiredResultsSucceeds(t *testing.T) {
	fixture := newServiceFixture()
	fixture.results.expiredRows = 3

	require.NoError(t, fixture.service.CleanupExpiredResults(context.Background()))
	require.Equal(t, 1, fixture.results.expiredCalls)
}

func TestCleanupExpiredResultsReturnsRepositoryError(t *testing.T) {
	fixture := newServiceFixture()
	cause := errors.New("db down")
	fixture.results.expiredErr = cause

	require.ErrorIs(t, fixture.service.CleanupExpiredResults(context.Background()), cause)
}
