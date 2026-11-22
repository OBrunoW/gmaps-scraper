package application

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const (
	defaultFlushGrace       = 90 * time.Second
	flushWaitWarnThreshold  = 20 * time.Second
	jobRuntimeWarnThreshold = 4*time.Minute + 30*time.Second
)

type WatchdogMetrics struct {
	FlushWaitWarnTotal   int64
	LongRuntimeWarnTotal int64
}

type ScrapeRunner struct {
	scraper                domain.Scraper
	flushGrace             time.Duration
	flushWaitWarnThreshold time.Duration
	runtimeWarnThreshold   time.Duration
	flushWaitWarnTotal     atomic.Int64
	longRuntimeWarnTotal   atomic.Int64
}

func NewScrapeRunner(scraper domain.Scraper, flushGrace time.Duration) *ScrapeRunner {
	return &ScrapeRunner{
		scraper:                scraper,
		flushGrace:             flushGrace,
		flushWaitWarnThreshold: flushWaitWarnThreshold,
		runtimeWarnThreshold:   jobRuntimeWarnThreshold,
	}
}

func (r *ScrapeRunner) Metrics() WatchdogMetrics {
	return WatchdogMetrics{
		FlushWaitWarnTotal:   r.flushWaitWarnTotal.Load(),
		LongRuntimeWarnTotal: r.longRuntimeWarnTotal.Load(),
	}
}

func (r *ScrapeRunner) Run(ctx context.Context, job domain.ScrapeJob) error {
	defer r.scraper.JobDone()

	jobStart := time.Now()

	defer func() {
		runtime := time.Since(jobStart)
		if runtime > r.runtimeWarnThreshold {
			r.longRuntimeWarnTotal.Add(1)
			log.Warn("scrape job exceeded watchdog runtime",
				"job_id", job.ID,
				"runtime_ms", runtime.Milliseconds(),
				"threshold_ms", r.runtimeWarnThreshold.Milliseconds(),
			)
		}
	}()

	timeout := domain.EffectiveScrapeTimeout(job.Params.TimeoutSecs)

	completionCh := r.scraper.Register(job)
	flushWaitStart := time.Now()

	stop, err := r.scraper.Submit(ctx, job)
	if err != nil {
		r.scraper.ForceFlush(job)

		if _, waitErr := r.waitForFlushResult(completionCh); waitErr != nil {
			log.Warn("failed waiting for flush result after submit failure",
				"job_id", job.ID,
				"wait_error", waitErr,
			)
		}

		return fmt.Errorf("failed to submit job: %w", err)
	}

	defer stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-completionCh:
		r.observeFlushWait(job.ID, time.Since(flushWaitStart))
		log.Debug("job flush completed",
			"job_id", job.ID,
			"result_count", result.ResultCount,
			"wait_ms", time.Since(flushWaitStart).Milliseconds(),
		)

		return result.Err
	case <-timer.C:
		return r.finishAfterTimer(job, completionCh, stop, flushWaitStart)
	case <-ctx.Done():
		return r.finishAfterContextDone(ctx, job, completionCh, stop, flushWaitStart)
	}
}

func (r *ScrapeRunner) finishAfterTimer(
	job domain.ScrapeJob,
	completionCh <-chan domain.FlushResult,
	stop func(),
	flushWaitStart time.Time,
) error {
	stop()
	r.scraper.ForceFlush(job)

	result, err := r.waitForFlushResult(completionCh)
	if err != nil {
		r.observeFlushWait(job.ID, time.Since(flushWaitStart))

		return err
	}

	r.observeFlushWait(job.ID, time.Since(flushWaitStart))

	if result.Err != nil {
		return result.Err
	}

	if result.ResultCount == 0 {
		log.Warn("job timed out with no results",
			"keyword", job.Params.Keyword,
			"attempt", job.Attempt,
			"job_id", job.ID,
		)

		return errors.New("job timed out with no results")
	}

	log.Info("job timed out but saved partial results",
		"keyword", job.Params.Keyword,
		"result_count", result.ResultCount,
		"job_id", job.ID,
	)

	return nil
}

func (r *ScrapeRunner) finishAfterContextDone(
	ctx context.Context,
	job domain.ScrapeJob,
	completionCh <-chan domain.FlushResult,
	stop func(),
	flushWaitStart time.Time,
) error {
	stop()
	r.scraper.ForceFlush(job)

	result, err := r.waitForFlushResult(completionCh)
	if err != nil {
		r.observeFlushWait(job.ID, time.Since(flushWaitStart))

		return err
	}

	r.observeFlushWait(job.ID, time.Since(flushWaitStart))

	if result.Err != nil {
		return result.Err
	}

	if ctx.Err() == context.DeadlineExceeded {
		if result.ResultCount > 0 {
			log.Info("job context expired but saved results",
				"keyword", job.Params.Keyword,
				"result_count", result.ResultCount,
				"job_id", job.ID,
			)

			return nil
		}

		return errors.New("job timed out with no results")
	}

	return ctx.Err()
}

func (r *ScrapeRunner) waitForFlushResult(ch <-chan domain.FlushResult) (domain.FlushResult, error) {
	return waitForFlushResult(ch, r.flushGrace)
}

func waitForFlushResult(ch <-chan domain.FlushResult, grace time.Duration) (domain.FlushResult, error) {
	if grace <= 0 {
		grace = defaultFlushGrace
	}

	timer := time.NewTimer(grace)
	defer timer.Stop()

	select {
	case result := <-ch:
		return result, nil
	case <-timer.C:
		return domain.FlushResult{}, errors.New("timed out waiting for flush result")
	}
}

func (r *ScrapeRunner) observeFlushWait(jobID int64, wait time.Duration) {
	if wait <= r.flushWaitWarnThreshold {
		return
	}

	r.flushWaitWarnTotal.Add(1)
	log.Warn("flush wait exceeded watchdog threshold",
		"job_id", jobID,
		"wait_ms", wait.Milliseconds(),
		"threshold_ms", r.flushWaitWarnThreshold.Milliseconds(),
	)
}
