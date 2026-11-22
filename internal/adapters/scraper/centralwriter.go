package scraper

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/adapters/gmaps"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/jsonbsanitize"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
	"github.com/gosom/scrapemate"
)

type trackedJob struct {
	jobID      string
	entries    []*gmaps.Entry
	completion chan domain.FlushResult
	riverJobID int64
	keyword    string
	startedAt  time.Time
}

type SaveFunc func(ctx context.Context, riverJobID int64, keyword string, entries []*gmaps.Entry) error

var _ scrapemate.ResultWriter = (*CentralWriter)(nil)

type CentralWriter struct {
	mu      sync.Mutex
	current *trackedJob

	save           SaveFunc
	OnResultsSaved func(count int)
}

func NewCentralWriter(repo domain.ResultRepository, saveFn SaveFunc) *CentralWriter {
	if saveFn == nil {
		saveFn = storeSave(repo)
	}

	return &CentralWriter{save: saveFn}
}

func (cw *CentralWriter) RegisterJob(jobID string, riverJobID int64, keyword string) <-chan domain.FlushResult {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	ch := make(chan domain.FlushResult, 1)
	cw.current = &trackedJob{
		jobID:      jobID,
		completion: ch,
		riverJobID: riverJobID,
		keyword:    keyword,
		startedAt:  time.Now(),
	}

	log.Debug("registered scrape job", "job_id", jobID, "river_job_id", riverJobID)

	return ch
}

func (cw *CentralWriter) AddResult(jobID string, entry *gmaps.Entry) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	if cw.current == nil || cw.current.jobID != jobID {
		return
	}

	cw.current.entries = append(cw.current.entries, entry)
}

func (cw *CentralWriter) MarkDone(jobID string) {
	cw.Flush(jobID)
}

func (cw *CentralWriter) ForceFlush(jobID string) {
	cw.Flush(jobID)
}

func (cw *CentralWriter) Discard(jobID string) {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	if cw.current != nil && cw.current.jobID == jobID {
		cw.current = nil
	}
}

func (cw *CentralWriter) TrackedJobs() int {
	cw.mu.Lock()
	defer cw.mu.Unlock()

	if cw.current == nil {
		return 0
	}

	return 1
}

func (cw *CentralWriter) FlushQueueDepth() int {
	return 0
}

func (cw *CentralWriter) Flush(jobID string) {
	cw.mu.Lock()
	j := cw.current

	if j == nil || j.jobID != jobID {
		cw.mu.Unlock()
		return
	}

	cw.current = nil
	cw.mu.Unlock()

	for _, entry := range j.entries {
		jsonbsanitize.StripNULFromEntry(entry)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	err := cw.save(ctx, j.riverJobID, j.keyword, j.entries)

	cancel()

	if err != nil {
		log.Error("failed to save results",
			"job_id", j.jobID,
			"river_job_id", j.riverJobID,
			"error", err,
		)
	} else if cw.OnResultsSaved != nil && len(j.entries) > 0 {
		cw.OnResultsSaved(len(j.entries))
	}

	j.completion <- domain.FlushResult{ResultCount: len(j.entries), Err: err}

	log.Debug("flushed scrape job",
		"job_id", j.jobID,
		"river_job_id", j.riverJobID,
		"result_count", len(j.entries),
		"duration_ms", time.Since(j.startedAt).Milliseconds(),
		"save_error", err,
	)
}

func (cw *CentralWriter) Run(ctx context.Context, in <-chan scrapemate.Result) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case result, ok := <-in:
			if !ok {
				return nil
			}

			cw.processResult(result)
		}
	}
}

func (cw *CentralWriter) processResult(result scrapemate.Result) {
	jobID := ""
	if result.Job != nil {
		jobID = result.Job.GetID()
	}

	if entry, ok := result.Data.(*gmaps.Entry); ok {
		entryJobID := entry.ID
		if entryJobID == "" {
			entryJobID = jobID
		}

		cw.AddResult(entryJobID, entry)
		cw.markCompletedFromResult(result, 1)

		return
	}

	if entries, ok := result.Data.([]*gmaps.Entry); ok {
		for _, entry := range entries {
			entry.ID = jobID
			cw.AddResult(jobID, entry)
		}

		cw.markCompletedFromResult(result, len(entries))
	}
}

func (cw *CentralWriter) markCompletedFromResult(result scrapemate.Result, count int) {
	if count <= 0 || result.Job == nil {
		return
	}

	switch job := result.Job.(type) {
	case *gmaps.PlaceJob:
		if job.WriterManagedCompletion && job.ExitMonitor != nil {
			job.ExitMonitor.IncrPlacesCompleted(count)
		}
	case *gmaps.EmailExtractJob:
		if job.WriterManagedCompletion && job.ExitMonitor != nil {
			job.ExitMonitor.IncrPlacesCompleted(count)
		}
	case *gmaps.SearchJob:
		if job.WriterManagedCompletion && job.ExitMonitor != nil {
			job.ExitMonitor.IncrPlacesCompleted(count)
		}
	}
}

func storeSave(repo domain.ResultRepository) SaveFunc {
	return func(ctx context.Context, riverJobID int64, keyword string, entries []*gmaps.Entry) error {
		resultsJSON, err := json.Marshal(entries)
		if err != nil {
			return err
		}

		return repo.Save(ctx, domain.ScrapeResult{
			JobID:       riverJobID,
			Keyword:     keyword,
			Results:     resultsJSON,
			ResultCount: len(entries),
		})
	}
}
