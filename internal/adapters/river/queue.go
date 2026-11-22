package river

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const defaultListLimit = 20

type Queue struct {
	client *river.Client[pgx.Tx]
}

func newQueue(client *river.Client[pgx.Tx]) *Queue {
	return &Queue{client: client}
}

func (q *Queue) Enqueue(ctx context.Context, params domain.ScrapeParams) (int64, error) {
	result, err := q.client.Insert(ctx, argsFromParams(params), nil)
	if err != nil {
		return 0, fmt.Errorf("insert scrape job: %w", err)
	}

	return result.Job.ID, nil
}

func (q *Queue) Get(ctx context.Context, id int64) (domain.JobRecord, error) {
	job, err := q.client.JobGet(ctx, id)
	if err != nil {
		return domain.JobRecord{}, fmt.Errorf("get job %d: %w", id, err)
	}

	return toRecord(job), nil
}

func (q *Queue) List(ctx context.Context, filter domain.JobListFilter) (domain.JobPage, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}

	params := river.NewJobListParams().Kinds(KindScrape).First(limit)

	if filter.State != "" {
		params = params.States(rivertype.JobState(filter.State))
	}

	if filter.Cursor != "" {
		var cursor river.JobListCursor
		if err := cursor.UnmarshalText([]byte(filter.Cursor)); err != nil {
			return domain.JobPage{}, domain.InvalidInput("invalid cursor: " + err.Error())
		}

		params = params.After(&cursor)
	}

	result, err := q.client.JobList(ctx, params)
	if err != nil {
		return domain.JobPage{}, fmt.Errorf("list jobs: %w", err)
	}

	page := domain.JobPage{Records: make([]domain.JobRecord, 0, len(result.Jobs))}
	for _, job := range result.Jobs {
		page.Records = append(page.Records, toRecord(job))
	}

	if result.LastCursor != nil {
		if text, err := result.LastCursor.MarshalText(); err == nil {
			page.NextCursor = string(text)
		}
	}

	return page, nil
}

func (q *Queue) Cancel(ctx context.Context, id int64) error {
	if _, err := q.client.JobCancel(ctx, id); err != nil {
		return fmt.Errorf("cancel job %d: %w", id, err)
	}

	return nil
}

func (q *Queue) EnqueuePurge(ctx context.Context, id int64) error {
	if _, err := q.client.Insert(ctx, JobDeleteArgs{JobID: id}, nil); err != nil {
		return fmt.Errorf("insert purge job %d: %w", id, err)
	}

	return nil
}

func toRecord(job *rivertype.JobRow) domain.JobRecord {
	record := domain.JobRecord{
		ID:          job.ID,
		State:       domain.JobState(job.State),
		CreatedAt:   job.CreatedAt,
		AttemptedAt: job.AttemptedAt,
		FinalizedAt: job.FinalizedAt,
	}

	var args ScrapeJobArgs
	if err := json.Unmarshal(job.EncodedArgs, &args); err == nil {
		record.Keyword = args.Keyword
	}

	if len(job.Errors) > 0 {
		record.LastError = job.Errors[len(job.Errors)-1].Error
	}

	return record
}
