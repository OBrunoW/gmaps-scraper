package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/middleware"
)

type fakeQueue struct {
	enqueueID    int64
	enqueueErr   error
	enqueued     []domain.ScrapeParams
	records      map[int64]domain.JobRecord
	getErr       error
	page         domain.JobPage
	listErr      error
	listFilter   domain.JobListFilter
	purgeErr     error
	purgedIDs    []int64
	cancelledIDs []int64
}

func (f *fakeQueue) Enqueue(_ context.Context, params domain.ScrapeParams) (int64, error) {
	f.enqueued = append(f.enqueued, params)
	return f.enqueueID, f.enqueueErr
}

func (f *fakeQueue) Get(_ context.Context, id int64) (domain.JobRecord, error) {
	if f.getErr != nil {
		return domain.JobRecord{}, f.getErr
	}
	record, ok := f.records[id]
	if !ok {
		return domain.JobRecord{}, errors.New("not found")
	}
	return record, nil
}

func (f *fakeQueue) List(_ context.Context, filter domain.JobListFilter) (domain.JobPage, error) {
	f.listFilter = filter
	return f.page, f.listErr
}

func (f *fakeQueue) Cancel(_ context.Context, id int64) error {
	f.cancelledIDs = append(f.cancelledIDs, id)
	return nil
}

func (f *fakeQueue) EnqueuePurge(_ context.Context, id int64) error {
	f.purgedIDs = append(f.purgedIDs, id)
	return f.purgeErr
}

type fakeResults struct {
	result domain.ScrapeResult
	err    error
	counts map[int64]int
}

func (f *fakeResults) Save(context.Context, domain.ScrapeResult) error { return nil }

func (f *fakeResults) FindByJobID(context.Context, int64) (domain.ScrapeResult, error) {
	return f.result, f.err
}

func (f *fakeResults) CountByJobIDs(context.Context, []int64) (map[int64]int, error) {
	return f.counts, nil
}

func (f *fakeResults) DeleteByJobID(context.Context, int64) (int64, error) { return 0, nil }

func (f *fakeResults) DeleteExpired(context.Context) (int64, error) { return 0, nil }

type fakeJobs struct{}

func (fakeJobs) Delete(context.Context, int64) (int64, error) { return 0, nil }

type fakeCodec struct{}

func (fakeCodec) Encode(id int64) string { return "pub" + strconv.FormatInt(id, 10) }

func (fakeCodec) Decode(publicID string) (int64, error) {
	raw, ok := strings.CutPrefix(publicID, "pub")
	if !ok {
		return 0, errors.New("malformed id")
	}
	return strconv.ParseInt(raw, 10, 64)
}

type testEnv struct {
	queue   *fakeQueue
	results *fakeResults
	router  http.Handler
}

func newTestEnv(auth func(http.Handler) http.Handler) *testEnv {
	queue := &fakeQueue{enqueueID: 7, records: map[int64]domain.JobRecord{}}
	results := &fakeResults{}
	service := application.NewJobService(queue, results, fakeJobs{}, fakeCodec{})
	if auth == nil {
		auth = middleware.NoopAuth
	}
	return &testEnv{queue: queue, results: results, router: NewRouter(service, auth)}
}
