package river

import (
	"context"
	"fmt"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

const (
	retryPromoteInterval = 10 * time.Second
	retryPromoteLimit    = 1
	retryPromoteMaxBatch = 50
	rescueStuckAfter     = 20 * time.Minute
	cleanupInterval      = time.Hour
	maintenanceWorkers   = 2
	scrapeWorkers        = 1
)

type Client struct {
	riverClient *river.Client[pgx.Tx]
	maintenance *maintenanceRef
}

func NewClient(dbPool *pgxpool.Pool) (*Client, error) {
	ref := &maintenanceRef{}

	workers := river.NewWorkers()
	river.AddWorker(workers, &ScrapeWorker{})
	river.AddWorker(workers, &JobDeleteWorker{ref: ref})
	river.AddWorker(workers, &CleanupWorker{ref: ref})

	periodicJobs := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(cleanupInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return CleanupArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
	}

	riverClient, err := river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Queues: map[string]river.QueueConfig{
			QueueMaintenance: {MaxWorkers: maintenanceWorkers},
		},
		Workers:              workers,
		PeriodicJobs:         periodicJobs,
		Logger:               log.With("component", "river-server"),
		RescueStuckJobsAfter: rescueStuckAfter,
	})
	if err != nil {
		return nil, fmt.Errorf("create river serve client: %w", err)
	}

	return &Client{riverClient: riverClient, maintenance: ref}, nil
}

func NewWorkerClient(dbPool *pgxpool.Pool, runner *application.ScrapeRunner) (*Client, error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, &ScrapeWorker{runner: runner})

	riverClient, err := river.NewClient(riverpgxv5.New(dbPool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: scrapeWorkers},
		},
		Workers:              workers,
		Logger:               log.With("component", "river-worker"),
		JobTimeout:           domain.MaxScrapeTimeout + timeoutMargin,
		RescueStuckJobsAfter: rescueStuckAfter,
	})
	if err != nil {
		return nil, fmt.Errorf("create river worker client: %w", err)
	}

	return &Client{riverClient: riverClient, maintenance: &maintenanceRef{}}, nil
}

func (c *Client) SetMaintenance(service Maintenance) {
	c.maintenance.service = service
}

func (c *Client) Queue() *Queue {
	return newQueue(c.riverClient)
}

func (c *Client) Start(ctx context.Context) error {
	return c.riverClient.Start(ctx)
}

func (c *Client) Stop(ctx context.Context) error {
	return c.riverClient.Stop(ctx)
}

func (c *Client) StartRetryPromoter(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(retryPromoteInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.promoteRetryableJobs(ctx, retryPromoteLimit)
			}
		}
	}()
}

func (c *Client) promoteRetryableJobs(ctx context.Context, limit int) {
	if limit <= 0 {
		return
	}

	if limit > retryPromoteMaxBatch {
		limit = retryPromoteMaxBatch
	}

	jobs, err := c.riverClient.JobList(ctx,
		river.NewJobListParams().
			Kinds(KindScrape).
			States(rivertype.JobStateRetryable).
			First(limit),
	)
	if err != nil {
		return
	}

	for _, job := range jobs.Jobs {
		if _, err := c.riverClient.JobRetry(ctx, job.ID); err != nil {
			log.Error("failed to promote retryable job", "job_id", job.ID, "error", err)
			continue
		}

		log.Info("promoted retryable job for immediate retry",
			"job_id", job.ID,
			"attempt", job.Attempt,
		)
	}
}
