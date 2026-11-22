package river

import (
	"context"
	"errors"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/riverqueue/river"
)

const (
	retryFallbackDelay = 2 * time.Minute
	timeoutMargin      = 2 * time.Minute
)

var errRunnerNotConfigured = errors.New("scrape runner is not configured")

type ScrapeWorker struct {
	river.WorkerDefaults[ScrapeJobArgs]
	runner *application.ScrapeRunner
}

func (w *ScrapeWorker) NextRetryAt(_ *river.Job[ScrapeJobArgs]) time.Time {
	return time.Now().Add(retryFallbackDelay)
}

func (w *ScrapeWorker) Timeout(job *river.Job[ScrapeJobArgs]) time.Duration {
	timeout := domain.MaxScrapeTimeout
	if job != nil {
		timeout = domain.EffectiveScrapeTimeout(job.Args.TimeoutSecs)
	}

	return timeout + timeoutMargin
}

func (w *ScrapeWorker) Work(ctx context.Context, job *river.Job[ScrapeJobArgs]) error {
	if w.runner == nil {
		return errRunnerNotConfigured
	}

	return w.runner.Run(ctx, domain.ScrapeJob{
		ID:      job.ID,
		Attempt: job.Attempt,
		Params:  job.Args.params(),
	})
}
