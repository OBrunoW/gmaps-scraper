package httpadapter

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
)

const (
	maxRequestBodyBytes = 1 << 20
	defaultListLimit    = 20
)

type Handlers struct {
	jobs *application.JobService
}

func NewHandlers(jobs *application.JobService) *Handlers {
	return &Handlers{jobs: jobs}
}

func (h *Handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthCheckResponse{Status: "ok"})
}

func (h *Handlers) createJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var request ScrapeRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, domain.InvalidInput("invalid request body"))
		return
	}

	created, err := h.jobs.Create(r.Context(), request.toParams())
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, ScrapeResponse{
		JobID:  created.PublicID,
		Status: string(created.Status),
	})
}

func (h *Handlers) listJobs(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	list, err := h.jobs.List(r.Context(), application.ListJobsInput{
		State:  query.Get("state"),
		Limit:  parseLimit(query.Get("limit")),
		Cursor: query.Get("cursor"),
	})
	if err != nil {
		writeError(w, err)
		return
	}

	summaries := make([]JobSummary, 0, len(list.Jobs))
	for _, view := range list.Jobs {
		summaries = append(summaries, JobSummary{
			JobID:       view.PublicID,
			Status:      string(view.Status),
			Keyword:     view.Keyword,
			CreatedAt:   view.CreatedAt,
			StartedAt:   view.StartedAt,
			CompletedAt: view.CompletedAt,
			ResultCount: view.ResultCount,
			Error:       view.Error,
		})
	}

	writeJSON(w, http.StatusOK, ListJobsResponse{
		Jobs:       summaries,
		NextCursor: list.NextCursor,
		HasMore:    list.HasMore,
	})
}

func (h *Handlers) getJob(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "job_id")

	view, err := h.jobs.Get(r.Context(), publicID)
	if err != nil {
		writeError(w, err)
		return
	}

	var results any
	if view.Results != nil {
		_ = json.Unmarshal(view.Results, &results)
	}

	writeJSON(w, http.StatusOK, JobStatusResponse{
		JobID:       publicID,
		Status:      string(view.Status),
		Keyword:     view.Keyword,
		CreatedAt:   view.CreatedAt,
		StartedAt:   view.StartedAt,
		CompletedAt: view.CompletedAt,
		Results:     results,
		Error:       view.Error,
		ResultCount: view.ResultCount,
	})
}

func (h *Handlers) deleteJob(w http.ResponseWriter, r *http.Request) {
	if err := h.jobs.Delete(r.Context(), chi.URLParam(r, "job_id")); err != nil {
		writeError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, DeleteJobResponse{Message: "deletion queued"})
}

func parseLimit(raw string) int {
	if raw == "" {
		return defaultListLimit
	}

	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return defaultListLimit
	}

	return limit
}
