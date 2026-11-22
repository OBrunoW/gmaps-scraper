package scraper

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/adapters/gmaps"
	"github.com/OBrunoW/gmaps-scraper/internal/adapters/gmaps/exiter"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/scrapemateapp"
)

//nolint:revive // ScraperManager is intentionally named with its package context
type ScraperManager struct {
	results     domain.ResultRepository
	concurrency int
	fastMode    bool
	debug       bool
	maxJobs     int64
	proxies     []string

	mu       sync.RWMutex
	provider *Provider

	centralWriter *CentralWriter

	jobCount atomic.Int64

	restartChan chan struct{}

	OnJobComplete func()
}

func NewScraperManager(results domain.ResultRepository, concurrency int, fastMode, debug bool, maxJobs int64, proxies []string) *ScraperManager {
	if concurrency <= 0 {
		concurrency = 1
	}

	if maxJobs <= 0 {
		maxJobs = math.MaxInt64
	}

	return &ScraperManager{
		results:       results,
		concurrency:   concurrency,
		fastMode:      fastMode,
		debug:         debug,
		maxJobs:       maxJobs,
		proxies:       proxies,
		restartChan:   make(chan struct{}, 1),
		centralWriter: NewCentralWriter(results, nil),
	}
}

func (m *ScraperManager) CentralWriter() *CentralWriter {
	return m.centralWriter
}

func (m *ScraperManager) getProvider() *Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.provider
}

func (m *ScraperManager) JobDone() {
	count := m.jobCount.Add(1)

	if m.OnJobComplete != nil {
		m.OnJobComplete()
	}

	if count >= m.maxJobs {

		select {
		case m.restartChan <- struct{}{}:
		default:
		}
	}
}

func (m *ScraperManager) ActiveJobs() int64 {
	return int64(m.centralWriter.TrackedJobs())
}

func (m *ScraperManager) SubmitJob(ctx context.Context, job scrapemate.IJob) error {
	switch j := job.(type) {
	case *gmaps.GmapJob:
		if !j.WriterManagedCompletion {
			return fmt.Errorf("failed to submit job: GmapJob requires WriterManagedCompletion")
		}
	case *gmaps.SearchJob:
		if !j.WriterManagedCompletion {
			return fmt.Errorf("failed to submit job: SearchJob requires WriterManagedCompletion")
		}
	}

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		p := m.getProvider()
		if p != nil {
			return p.Submit(ctx, job)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to submit job: scraper provider not ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func (m *ScraperManager) RegisterJob(jobID string, riverJobID int64, keyword string) <-chan domain.FlushResult {
	return m.centralWriter.RegisterJob(jobID, riverJobID, keyword)
}

func (m *ScraperManager) Register(job domain.ScrapeJob) <-chan domain.FlushResult {
	return m.RegisterJob(strconv.FormatInt(job.ID, 10), job.ID, job.Params.Keyword)
}

func (m *ScraperManager) Submit(ctx context.Context, job domain.ScrapeJob) (func(), error) {
	jobID := strconv.FormatInt(job.ID, 10)

	exitMon := exiter.New()
	exitMon.SetSeedCount(1)
	exitMon.SetCancelFunc(func() { m.MarkDone(jobID) })

	if err := m.SubmitJob(ctx, buildScrapeJob(jobID, job.Params, exitMon)); err != nil {
		return nil, err
	}

	exitCtx, exitCancel := context.WithCancel(ctx)

	go exitMon.Run(exitCtx)

	return exitCancel, nil
}

func buildScrapeJob(jobID string, params domain.ScrapeParams, exitMon exiter.Exiter) scrapemate.IJob {
	if params.FastMode {
		searchParams := &gmaps.MapSearchParams{
			Query: params.Keyword,
			Hl:    params.Lang,
		}

		if params.GeoCoordinates != "" {
			lat, lon := parseGeoCoordinates(params.GeoCoordinates)
			searchParams.Location = gmaps.MapLocation{
				Lat:     lat,
				Lon:     lon,
				ZoomLvl: float64(params.Zoom),
				Radius:  params.Radius,
			}
		}

		searchJob := gmaps.NewSearchJob(searchParams,
			gmaps.WithSearchJobExitMonitor(exitMon),
			gmaps.WithSearchJobWriterManagedCompletion(),
		)
		searchJob.ID = jobID

		return searchJob
	}

	maxDepth := params.MaxDepth
	if maxDepth == 0 {
		maxDepth = 10
	}

	opts := []gmaps.GmapJobOptions{
		gmaps.WithExitMonitor(exitMon),
		gmaps.WithWriterManagedCompletion(),
	}

	if params.ExtraReviews {
		opts = append(opts, gmaps.WithExtraReviews())
	}

	return gmaps.NewGmapJob(
		jobID,
		params.Lang,
		params.Keyword,
		maxDepth,
		params.Email,
		params.GeoCoordinates,
		params.Zoom,
		opts...,
	)
}

func parseGeoCoordinates(coords string) (lat, lon float64) {
	parts := strings.Split(coords, ",")
	if len(parts) != 2 {
		return 0, 0
	}

	lat, _ = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, _ = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)

	return lat, lon
}

func (m *ScraperManager) MarkDone(jobID string) {
	m.centralWriter.MarkDone(jobID)
}

func (m *ScraperManager) ForceFlush(job domain.ScrapeJob) {
	m.centralWriter.ForceFlush(strconv.FormatInt(job.ID, 10))
}

func (m *ScraperManager) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if err := m.runCycle(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return err
		}
	}
}

func (m *ScraperManager) runCycle(ctx context.Context) error {
	provider := NewProvider(m.concurrency * 64)

	m.mu.Lock()
	m.provider = provider
	m.jobCount.Store(0)
	m.mu.Unlock()

	app, err := m.createApp(provider)
	if err != nil {
		return err
	}

	defer func() { _ = app.Close() }()

	cycleCtx, cycleCancel := context.WithCancel(ctx)
	defer cycleCancel()

	scraperDone := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				scraperDone <- fmt.Errorf("scraper panic: %v", r)
			}
		}()

		scraperDone <- app.Start(cycleCtx)
	}()

	log.Info("scraper cycle started", "max_jobs", m.maxJobs)

	select {
	case <-ctx.Done():
		cycleCancel()

		if err := <-scraperDone; err != nil {
			log.Warn("scraper exited during shutdown", "error", err)
		}

		return nil

	case err := <-scraperDone:

		if ctx.Err() != nil {
			return nil
		}

		return err

	case <-m.restartChan:
		log.Info("restart triggered, restarting scraper",
			"jobs_processed", m.jobCount.Load(),
		)

		cycleCancel()

		if err := <-scraperDone; err != nil {
			log.Warn("scraper exited during restart", "error", err)
		}

		return nil
	}
}

func (m *ScraperManager) createApp(provider *Provider) (*scrapemateapp.ScrapemateApp, error) {
	writers := []scrapemate.ResultWriter{m.centralWriter}

	var opts []func(*scrapemateapp.Config) error

	opts = append(opts,
		scrapemateapp.WithConcurrency(m.concurrency),
		scrapemateapp.WithProvider(provider),
	)

	if len(m.proxies) > 0 {
		opts = append(opts, scrapemateapp.WithProxies(m.proxies))
		log.Info("proxies configured", "count", len(m.proxies))
	}

	if m.fastMode {
		opts = append(opts, scrapemateapp.WithStealth("firefox"))
	} else {
		if m.debug {
			opts = append(opts, scrapemateapp.WithJS(
				scrapemateapp.Headfull(),
				scrapemateapp.DisableImages(),
			))
		} else {
			opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
		}
	}

	cfg, err := scrapemateapp.NewConfig(writers, opts...)
	if err != nil {
		return nil, err
	}

	return scrapemateapp.NewScrapeMateApp(cfg)
}

var _ domain.Scraper = (*ScraperManager)(nil)
