package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/stretchr/testify/require"
)

type fakeScraper struct {
	mu           sync.Mutex
	completion   chan domain.FlushResult
	submitErr    error
	submitDelay  time.Duration
	onSubmit     func()
	onForceFlush func(chan domain.FlushResult)
	registered   []domain.ScrapeJob
	submitted    []domain.ScrapeJob
	flushed      []domain.ScrapeJob
	stopCalls    int
	jobDoneCalls int
}

func newFakeScraper() *fakeScraper {
	return &fakeScraper{completion: make(chan domain.FlushResult, 1)}
}

func (s *fakeScraper) Register(job domain.ScrapeJob) <-chan domain.FlushResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.registered = append(s.registered, job)

	return s.completion
}

func (s *fakeScraper) Submit(_ context.Context, job domain.ScrapeJob) (func(), error) {
	time.Sleep(s.submitDelay)

	s.mu.Lock()
	s.submitted = append(s.submitted, job)
	s.mu.Unlock()

	if s.submitErr != nil {
		return nil, s.submitErr
	}

	if s.onSubmit != nil {
		s.onSubmit()
	}

	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()

		s.stopCalls++
	}, nil
}

func (s *fakeScraper) ForceFlush(job domain.ScrapeJob) {
	s.mu.Lock()
	s.flushed = append(s.flushed, job)
	s.mu.Unlock()

	if s.onForceFlush != nil {
		s.onForceFlush(s.completion)
	}
}

func (s *fakeScraper) JobDone() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.jobDoneCalls++
}

func (s *fakeScraper) flushCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.flushed)
}

func scrapeJobWithTimeout(timeoutSecs int) domain.ScrapeJob {
	return domain.ScrapeJob{
		ID:      11,
		Attempt: 1,
		Params:  domain.ScrapeParams{Keyword: "pizza", TimeoutSecs: timeoutSecs},
	}
}

func flushWith(result domain.FlushResult) func(chan domain.FlushResult) {
	return func(completion chan domain.FlushResult) { completion <- result }
}

func TestRunReturnsNilWhenFlushCompletes(t *testing.T) {
	scraper := newFakeScraper()
	scraper.onSubmit = func() { scraper.completion <- domain.FlushResult{ResultCount: 4} }
	runner := NewScrapeRunner(scraper, time.Second)

	err := runner.Run(context.Background(), scrapeJobWithTimeout(60))
	require.NoError(t, err)
	require.Equal(t, 1, scraper.jobDoneCalls)
	require.Equal(t, 1, scraper.stopCalls)
	require.Equal(t, []domain.ScrapeJob{scrapeJobWithTimeout(60)}, scraper.registered)
	require.Equal(t, []domain.ScrapeJob{scrapeJobWithTimeout(60)}, scraper.submitted)
	require.Zero(t, scraper.flushCount())
}

func TestRunReturnsFlushError(t *testing.T) {
	scraper := newFakeScraper()
	flushErr := errors.New("save failed")
	scraper.onSubmit = func() { scraper.completion <- domain.FlushResult{Err: flushErr} }
	runner := NewScrapeRunner(scraper, time.Second)

	err := runner.Run(context.Background(), scrapeJobWithTimeout(60))
	require.ErrorIs(t, err, flushErr)
	require.Equal(t, 1, scraper.jobDoneCalls)
}

func TestRunSubmitFailureFlushesAndWrapsError(t *testing.T) {
	scraper := newFakeScraper()
	submitErr := errors.New("provider closed")
	scraper.submitErr = submitErr
	scraper.onForceFlush = flushWith(domain.FlushResult{})
	runner := NewScrapeRunner(scraper, time.Second)

	err := runner.Run(context.Background(), scrapeJobWithTimeout(60))
	require.EqualError(t, err, "failed to submit job: provider closed")
	require.ErrorIs(t, err, submitErr)
	require.Equal(t, 1, scraper.flushCount())
	require.Equal(t, 1, scraper.jobDoneCalls)
	require.Zero(t, scraper.stopCalls)
}

func TestRunSubmitFailureStillReturnsWhenFlushNeverArrives(t *testing.T) {
	scraper := newFakeScraper()
	scraper.submitErr = errors.New("provider closed")
	runner := NewScrapeRunner(scraper, 30*time.Millisecond)

	err := runner.Run(context.Background(), scrapeJobWithTimeout(60))
	require.EqualError(t, err, "failed to submit job: provider closed")
	require.Equal(t, 1, scraper.flushCount())
}

func TestRunTimerWithPartialResultsSucceeds(t *testing.T) {
	scraper := newFakeScraper()
	scraper.onForceFlush = flushWith(domain.FlushResult{ResultCount: 6})
	runner := NewScrapeRunner(scraper, time.Second)

	err := runner.Run(context.Background(), scrapeJobWithTimeout(1))
	require.NoError(t, err)
	require.Equal(t, 1, scraper.flushCount())
	require.GreaterOrEqual(t, scraper.stopCalls, 1)
}

func TestRunTimerWithoutResultsFails(t *testing.T) {
	scraper := newFakeScraper()
	scraper.onForceFlush = flushWith(domain.FlushResult{})
	runner := NewScrapeRunner(scraper, time.Second)

	err := runner.Run(context.Background(), scrapeJobWithTimeout(1))
	require.EqualError(t, err, "job timed out with no results")
	require.Equal(t, 1, scraper.flushCount())
}

func TestRunTimerReturnsFlushError(t *testing.T) {
	scraper := newFakeScraper()
	flushErr := errors.New("save failed")
	scraper.onForceFlush = flushWith(domain.FlushResult{ResultCount: 3, Err: flushErr})
	runner := NewScrapeRunner(scraper, time.Second)

	err := runner.Run(context.Background(), scrapeJobWithTimeout(1))
	require.ErrorIs(t, err, flushErr)
}

func TestRunTimerReturnsGraceTimeoutWhenFlushNeverArrives(t *testing.T) {
	scraper := newFakeScraper()
	runner := NewScrapeRunner(scraper, 30*time.Millisecond)

	err := runner.Run(context.Background(), scrapeJobWithTimeout(1))
	require.EqualError(t, err, "timed out waiting for flush result")
}

func TestRunContextDeadlineWithResultsSucceeds(t *testing.T) {
	scraper := newFakeScraper()
	scraper.onForceFlush = flushWith(domain.FlushResult{ResultCount: 2})
	runner := NewScrapeRunner(scraper, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := runner.Run(ctx, scrapeJobWithTimeout(60))
	require.NoError(t, err)
	require.Equal(t, 1, scraper.flushCount())
}

func TestRunContextDeadlineWithoutResultsFails(t *testing.T) {
	scraper := newFakeScraper()
	scraper.onForceFlush = flushWith(domain.FlushResult{})
	runner := NewScrapeRunner(scraper, time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := runner.Run(ctx, scrapeJobWithTimeout(60))
	require.EqualError(t, err, "job timed out with no results")
}

func TestRunContextCancelReturnsContextError(t *testing.T) {
	scraper := newFakeScraper()
	scraper.onForceFlush = flushWith(domain.FlushResult{ResultCount: 5})
	runner := NewScrapeRunner(scraper, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)

	err := runner.Run(ctx, scrapeJobWithTimeout(60))
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, scraper.flushCount())
}

func TestRunContextDoneReturnsFlushError(t *testing.T) {
	scraper := newFakeScraper()
	flushErr := errors.New("save failed")
	scraper.onForceFlush = flushWith(domain.FlushResult{Err: flushErr})
	runner := NewScrapeRunner(scraper, time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)

	err := runner.Run(ctx, scrapeJobWithTimeout(60))
	require.ErrorIs(t, err, flushErr)
}

func TestRunContextDoneReturnsGraceTimeoutWhenFlushNeverArrives(t *testing.T) {
	scraper := newFakeScraper()
	runner := NewScrapeRunner(scraper, 30*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)

	err := runner.Run(ctx, scrapeJobWithTimeout(60))
	require.EqualError(t, err, "timed out waiting for flush result")
}

func TestRunWaitsForSlowSubmit(t *testing.T) {
	scraper := newFakeScraper()
	scraper.submitDelay = 40 * time.Millisecond
	scraper.onSubmit = func() { scraper.completion <- domain.FlushResult{ResultCount: 1} }
	runner := NewScrapeRunner(scraper, time.Second)

	require.NoError(t, runner.Run(context.Background(), scrapeJobWithTimeout(60)))
}

func TestMetricsStartAtZero(t *testing.T) {
	runner := NewScrapeRunner(newFakeScraper(), time.Second)

	require.Equal(t, WatchdogMetrics{}, runner.Metrics())
}

func TestMetricsCountFlushWaitAboveThreshold(t *testing.T) {
	scraper := newFakeScraper()
	scraper.onSubmit = func() {
		time.AfterFunc(30*time.Millisecond, func() { scraper.completion <- domain.FlushResult{ResultCount: 1} })
	}
	runner := NewScrapeRunner(scraper, time.Second)
	runner.flushWaitWarnThreshold = 10 * time.Millisecond

	require.NoError(t, runner.Run(context.Background(), scrapeJobWithTimeout(60)))
	require.Equal(t, WatchdogMetrics{FlushWaitWarnTotal: 1}, runner.Metrics())
}

func TestMetricsCountLongRuntime(t *testing.T) {
	scraper := newFakeScraper()
	scraper.onSubmit = func() {
		time.AfterFunc(30*time.Millisecond, func() { scraper.completion <- domain.FlushResult{ResultCount: 1} })
	}
	runner := NewScrapeRunner(scraper, time.Second)
	runner.runtimeWarnThreshold = 10 * time.Millisecond

	require.NoError(t, runner.Run(context.Background(), scrapeJobWithTimeout(60)))
	require.Equal(t, int64(1), runner.Metrics().LongRuntimeWarnTotal)
}

func TestMetricsAreIndependentPerRunner(t *testing.T) {
	first := NewScrapeRunner(newFakeScraper(), time.Second)
	second := NewScrapeRunner(newFakeScraper(), time.Second)

	first.observeFlushWait(1, time.Hour)

	require.Equal(t, int64(1), first.Metrics().FlushWaitWarnTotal)
	require.Zero(t, second.Metrics().FlushWaitWarnTotal)
}

func TestObserveFlushWaitIgnoresWaitAtOrBelowThreshold(t *testing.T) {
	runner := NewScrapeRunner(newFakeScraper(), time.Second)

	runner.observeFlushWait(1, flushWaitWarnThreshold)

	require.Zero(t, runner.Metrics().FlushWaitWarnTotal)
}

func TestWaitForFlushResultReturnsResultWithinGrace(t *testing.T) {
	ch := make(chan domain.FlushResult, 1)

	go func() {
		time.Sleep(20 * time.Millisecond)

		ch <- domain.FlushResult{ResultCount: 3}
	}()

	result, err := waitForFlushResult(ch, 200*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, 3, result.ResultCount)
}

func TestWaitForFlushResultTimesOut(t *testing.T) {
	ch := make(chan domain.FlushResult)

	_, err := waitForFlushResult(ch, 30*time.Millisecond)
	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out waiting for flush result")
}
