package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const (
	defaultListLimit = 20
	maxListLimit     = 100
)

type CreatedJob struct {
	PublicID string
	Status   domain.JobStatus
}

type ListJobsInput struct {
	State  string
	Limit  int
	Cursor string
}

type JobView struct {
	PublicID    string
	Status      domain.JobStatus
	Keyword     string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Results     json.RawMessage
	ResultCount int
	Error       string
}

type JobList struct {
	Jobs       []JobView
	NextCursor string
	HasMore    bool
}

type JobService struct {
	queue   domain.JobQueue
	results domain.ResultRepository
	jobs    domain.JobRepository
	codec   domain.JobIDCodec
}

func NewJobService(
	queue domain.JobQueue,
	results domain.ResultRepository,
	jobs domain.JobRepository,
	codec domain.JobIDCodec,
) *JobService {
	return &JobService{queue: queue, results: results, jobs: jobs, codec: codec}
}

func (s *JobService) Create(ctx context.Context, params domain.ScrapeParams) (CreatedJob, error) {
	params.SetDefaults()

	if err := params.Validate(); err != nil {
		return CreatedJob{}, err
	}

	id, err := s.queue.Enqueue(ctx, params)
	if err != nil {
		log.Error("failed to insert job", "error", err)
		return CreatedJob{}, domain.Internal("failed to create job", err)
	}

	return CreatedJob{PublicID: s.codec.Encode(id), Status: domain.StatusPending}, nil
}

func (s *JobService) List(ctx context.Context, input ListJobsInput) (JobList, error) {
	limit := clampListLimit(input.Limit)

	filter := domain.JobListFilter{Limit: limit, Cursor: input.Cursor}

	if input.State != "" {
		state, ok := domain.ListableStates[input.State]
		if !ok {
			return JobList{}, domain.InvalidInput("invalid state: " + input.State)
		}

		filter.State = state
	}

	page, err := s.queue.List(ctx, filter)
	if err != nil {
		return JobList{}, listError(err)
	}

	counts := s.countResults(ctx, page.Records)

	views := make([]JobView, 0, len(page.Records))
	for _, record := range page.Records {
		views = append(views, s.listView(record, counts))
	}

	return JobList{
		Jobs:       views,
		NextCursor: page.NextCursor,
		HasMore:    len(page.Records) == limit,
	}, nil
}

func (s *JobService) Get(ctx context.Context, publicID string) (JobView, error) {
	id, err := s.codec.Decode(publicID)
	if err != nil {
		return JobView{}, domain.NotFound("job not found")
	}

	record, err := s.queue.Get(ctx, id)
	if err != nil {
		return JobView{}, domain.NotFound("job not found")
	}

	view := baseView(publicID, record)

	switch record.State {
	case domain.StateAvailable, domain.StateScheduled, domain.StateRetryable, domain.StatePending:
		view.Status = domain.StatusPending
	case domain.StateRunning:
		view.Status = domain.StatusRunning
		view.StartedAt = record.AttemptedAt
	case domain.StateCompleted:
		view.Status = domain.StatusCompleted
		view.StartedAt = record.AttemptedAt
		view.CompletedAt = record.FinalizedAt

		if result, err := s.results.FindByJobID(ctx, id); err == nil {
			view.Results = result.Results
			view.ResultCount = result.ResultCount
		}
	case domain.StateCancelled, domain.StateDiscarded:
		view.Status = domain.StatusFailed
		view.Error = record.LastError
	}

	return view, nil
}

func (s *JobService) Delete(ctx context.Context, publicID string) error {
	id, err := s.codec.Decode(publicID)
	if err != nil {
		return domain.Internal("invalid job id: "+err.Error(), err)
	}

	record, err := s.queue.Get(ctx, id)
	if err != nil {
		return domain.NotFound("job not found")
	}

	if record.State == domain.StateRunning {
		_ = s.queue.Cancel(ctx, id)
	}

	if err := s.queue.EnqueuePurge(ctx, id); err != nil {
		return domain.Internal("failed to queue deletion: "+err.Error(), err)
	}

	return nil
}

func (s *JobService) Purge(ctx context.Context, id int64) error {
	log.Info("deleting job", "job_id", id)

	rows, err := s.results.DeleteByJobID(ctx, id)
	if err != nil {
		log.Error("failed to delete results", "error", err, "job_id", id)
		return fmt.Errorf("failed to delete results: %w", err)
	}

	log.Info("deleted scrape results", "job_id", id, "rows_affected", rows)

	rows, err = s.jobs.Delete(ctx, id)
	if err != nil {
		log.Error("failed to delete river job", "error", err, "job_id", id)
		return fmt.Errorf("failed to delete job: %w", err)
	}

	log.Info("deleted river job", "job_id", id, "rows_affected", rows)

	return nil
}

func (s *JobService) CleanupExpiredResults(ctx context.Context) error {
	rows, err := s.results.DeleteExpired(ctx)
	if err != nil {
		log.Error("cleanup: failed to delete old scrape results", "error", err)
		return err
	}

	if rows > 0 {
		log.Info("cleanup: deleted old scrape results", "count", rows)
	}

	return nil
}

func (s *JobService) countResults(ctx context.Context, records []domain.JobRecord) map[int64]int {
	ids := make([]int64, 0, len(records))

	for _, record := range records {
		if record.State == domain.StateCompleted {
			ids = append(ids, record.ID)
		}
	}

	if len(ids) == 0 {
		return map[int64]int{}
	}

	counts, err := s.results.CountByJobIDs(ctx, ids)
	if err != nil {
		return map[int64]int{}
	}

	return counts
}

func (s *JobService) listView(record domain.JobRecord, counts map[int64]int) JobView {
	view := baseView(s.codec.Encode(record.ID), record)

	switch record.State {
	case domain.StateAvailable, domain.StateScheduled, domain.StateRetryable, domain.StatePending:
		view.Status = domain.StatusPending
	case domain.StateRunning:
		view.Status = domain.StatusRunning
		view.StartedAt = record.AttemptedAt
	case domain.StateCompleted:
		view.Status = domain.StatusCompleted
		view.StartedAt = record.AttemptedAt
		view.CompletedAt = record.FinalizedAt
		view.ResultCount = counts[record.ID]
	case domain.StateCancelled, domain.StateDiscarded:
		view.Status = domain.StatusFailed
		view.Error = record.LastError
	}

	return view
}

func baseView(publicID string, record domain.JobRecord) JobView {
	return JobView{
		PublicID:  publicID,
		Keyword:   record.Keyword,
		CreatedAt: record.CreatedAt,
	}
}

func clampListLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}

	if limit > maxListLimit {
		return maxListLimit
	}

	return limit
}

func listError(err error) error {
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return appErr
	}

	invalid := domain.InvalidInput(err.Error())
	invalid.Err = err

	return invalid
}
