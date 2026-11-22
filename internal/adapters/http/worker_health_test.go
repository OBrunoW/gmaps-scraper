package httpadapter

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"
)

type stubWorkerStats struct {
	uptime time.Duration
}

func (s stubWorkerStats) Uptime() time.Duration     { return s.uptime }
func (stubWorkerStats) JobsProcessed() int64        { return 4 }
func (stubWorkerStats) ResultsCollected() int64     { return 120 }
func (stubWorkerStats) Concurrency() int            { return 2 }
func (stubWorkerStats) ActiveJobs() int64           { return 3 }
func (stubWorkerStats) FlushWaitWarnTotal() int64   { return 5 }
func (stubWorkerStats) LongRuntimeWarnTotal() int64 { return 6 }

func serveWorkerHealth(uptime time.Duration) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	NewWorkerHealthHandler(stubWorkerStats{uptime: uptime})(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	return recorder
}

func TestWorkerHealthPayload(t *testing.T) {
	recorder := serveWorkerHealth(90*time.Minute + 700*time.Millisecond)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("content type = %q", contentType)
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}

	want := map[string]any{
		"status":             "ok",
		"worker":             "running",
		"uptime":             "1h 30m",
		"jobs_processed":     float64(4),
		"results_collected":  float64(120),
		"results_per_minute": float64(120) / 90,
		"concurrency":        float64(2),
		"river_max_workers":  float64(1),
		"active_jobs":        float64(3),
		"watchdog": map[string]any{
			"flush_wait_warn_total":   float64(5),
			"long_runtime_warn_total": float64(6),
		},
	}
	if !reflect.DeepEqual(payload, want) {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestWorkerHealthKeys(t *testing.T) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(serveWorkerHealth(time.Minute).Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}

	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	want := []string{"active_jobs", "concurrency", "jobs_processed", "results_collected", "results_per_minute", "river_max_workers", "status", "uptime", "watchdog", "worker"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("keys = %v", keys)
	}
}

func TestWorkerHealthZeroUptimeHasNoRate(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal(serveWorkerHealth(0).Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["results_per_minute"] != float64(0) || payload["uptime"] != "0m" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := map[time.Duration]string{
		5 * time.Minute:               "5m",
		3*time.Hour + 2*time.Minute:   "3h 2m",
		49*time.Hour + 15*time.Minute: "2d 1h 15m",
	}
	for duration, want := range tests {
		if got := formatDuration(duration); got != want {
			t.Fatalf("formatDuration(%v) = %q, want %q", duration, got, want)
		}
	}
}
