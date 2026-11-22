package httpadapter

import (
	"fmt"
	"net/http"
	"time"
)

const riverMaxWorkers = 1

type WorkerStats interface {
	Uptime() time.Duration
	JobsProcessed() int64
	ResultsCollected() int64
	Concurrency() int
	ActiveJobs() int64
	FlushWaitWarnTotal() int64
	LongRuntimeWarnTotal() int64
}

type watchdogHealth struct {
	FlushWaitWarnTotal   int64 `json:"flush_wait_warn_total"`
	LongRuntimeWarnTotal int64 `json:"long_runtime_warn_total"`
}

type workerHealthResponse struct {
	Status           string         `json:"status"`
	Worker           string         `json:"worker"`
	Uptime           string         `json:"uptime"`
	JobsProcessed    int64          `json:"jobs_processed"`
	ResultsCollected int64          `json:"results_collected"`
	ResultsPerMinute float64        `json:"results_per_minute"`
	Concurrency      int            `json:"concurrency"`
	RiverMaxWorkers  int            `json:"river_max_workers"`
	ActiveJobs       int64          `json:"active_jobs"`
	Watchdog         watchdogHealth `json:"watchdog"`
}

func NewWorkerHealthHandler(stats WorkerStats) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		uptime := stats.Uptime().Truncate(time.Second)
		totalResults := stats.ResultsCollected()

		var resultsPerMinute float64
		if uptime.Minutes() > 0 {
			resultsPerMinute = float64(totalResults) / uptime.Minutes()
		}

		writeJSON(w, http.StatusOK, workerHealthResponse{
			Status:           "ok",
			Worker:           "running",
			Uptime:           formatDuration(uptime),
			JobsProcessed:    stats.JobsProcessed(),
			ResultsCollected: totalResults,
			ResultsPerMinute: resultsPerMinute,
			Concurrency:      stats.Concurrency(),
			RiverMaxWorkers:  riverMaxWorkers,
			ActiveJobs:       stats.ActiveJobs(),
			Watchdog: watchdogHealth{
				FlushWaitWarnTotal:   stats.FlushWaitWarnTotal(),
				LongRuntimeWarnTotal: stats.LongRuntimeWarnTotal(),
			},
		})
	}
}

func formatDuration(duration time.Duration) string {
	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}

	return fmt.Sprintf("%dm", minutes)
}
