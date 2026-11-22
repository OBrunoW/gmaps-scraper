package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/gosom/google-maps-scraper/httpext"
	"github.com/gosom/google-maps-scraper/log"
	"github.com/gosom/google-maps-scraper/rqueue"
)

type AppState struct {
	RQueue *rqueue.Client
	Auth   AuthConfig
}

func NewAppState(queue *rqueue.Client, auth AuthConfig) *AppState {
	return &AppState{
		RQueue: queue,
		Auth:   auth,
	}
}

func Routes(r chi.Router, appState *AppState) {
	r.Use(httpext.LoggingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	r.Get("/api/v1/health", healthCheckHandler(appState))

	r.Group(func(protected chi.Router) {
		protected.Use(KeyAuth(appState.Auth))
		protected.Post("/api/v1/scrape", scrapeHandler(appState))
		protected.Get("/api/v1/jobs", listJobsHandler(appState))
		protected.Get("/api/v1/jobs/{job_id}", getJobHandler(appState))
		protected.Delete("/api/v1/jobs/{job_id}", deleteJobHandler(appState))
	})
}

func healthCheckHandler(_ *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		respondWithJSON(w, http.StatusOK, HealthCheckResponse{
			Status: "ok",
		})
	}
}

func scrapeHandler(appState *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		var req ScrapeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondWithJSON(w, http.StatusBadRequest, ErrorResponse{
				Message: "invalid request body",
			})
			return
		}

		req.SetDefaults()

		if err := req.Validate(); err != nil {
			respondWithJSON(w, http.StatusBadRequest, ErrorResponse{
				Message: err.Error(),
			})
			return
		}

		jobArgs := rqueue.ScrapeJobArgs{
			Keyword:        req.Keyword,
			Lang:           req.Lang,
			MaxDepth:       req.MaxDepth,
			Email:          req.Email,
			GeoCoordinates: req.GeoCoordinates,
			Zoom:           req.Zoom,
			Radius:         req.Radius,
			FastMode:       req.FastMode,
			ExtraReviews:   req.ExtraReviews,
			TimeoutSecs:    req.Timeout,
		}

		jobID, err := appState.RQueue.InsertJob(r.Context(), jobArgs)
		if err != nil {
			log.Error("failed to insert job", "error", err, "job_id", jobID)
			respondWithJSON(w, http.StatusInternalServerError, ErrorResponse{
				Message: "failed to create job",
			})
			return
		}

		respondWithJSON(w, http.StatusAccepted, ScrapeResponse{
			JobID:  jobID,
			Status: "pending",
		})
	}
}

func listJobsHandler(appState *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		cursor := r.URL.Query().Get("cursor")

		limit := 20
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 100 {
			limit = 100
		}

		result, err := appState.RQueue.ListJobs(r.Context(), state, limit, cursor)
		if err != nil {
			respondWithJSON(w, http.StatusBadRequest, ErrorResponse{
				Message: err.Error(),
			})
			return
		}

		jobs := make([]JobSummary, 0, len(result.Jobs))
		for _, job := range result.Jobs {
			jobs = append(jobs, JobSummary{
				JobID:       job.JobID,
				Status:      job.Status,
				Keyword:     job.Keyword,
				CreatedAt:   job.CreatedAt,
				StartedAt:   job.StartedAt,
				CompletedAt: job.CompletedAt,
				ResultCount: job.ResultCount,
				Error:       job.Error,
			})
		}

		respondWithJSON(w, http.StatusOK, ListJobsResponse{
			Jobs:       jobs,
			NextCursor: result.NextCursor,
			HasMore:    result.HasMore,
		})
	}
}

func getJobHandler(appState *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "job_id")
		if jobID == "" {
			respondWithJSON(w, http.StatusBadRequest, ErrorResponse{
				Message: "job_id is required",
			})
			return
		}

		jobStatus, err := appState.RQueue.GetJobStatus(r.Context(), jobID)
		if err != nil {
			log.Error("failed to get job status", "error", err, "job_id", jobID)
			respondWithJSON(w, http.StatusNotFound, ErrorResponse{
				Message: "job not found",
			})
			return
		}

		var results any
		if jobStatus.Results != nil {
			_ = json.Unmarshal(jobStatus.Results, &results)
		}

		respondWithJSON(w, http.StatusOK, JobStatusResponse{
			JobID:       jobStatus.JobID,
			Status:      jobStatus.Status,
			Keyword:     jobStatus.Keyword,
			CreatedAt:   jobStatus.CreatedAt,
			StartedAt:   jobStatus.StartedAt,
			CompletedAt: jobStatus.CompletedAt,
			Results:     results,
			Error:       jobStatus.Error,
			ResultCount: jobStatus.ResultCount,
		})
	}
}

func deleteJobHandler(appState *AppState) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID := chi.URLParam(r, "job_id")
		if jobID == "" {
			respondWithJSON(w, http.StatusBadRequest, ErrorResponse{
				Message: "job_id is required",
			})
			return
		}

		err := appState.RQueue.DeleteJob(r.Context(), jobID)
		if err != nil {
			if containsNotFound(err) {
				respondWithJSON(w, http.StatusNotFound, ErrorResponse{
					Message: "job not found",
				})
				return
			}

			log.Error("failed to delete job", "error", err, "job_id", jobID)
			respondWithJSON(w, http.StatusInternalServerError, ErrorResponse{
				Message: err.Error(),
			})
			return
		}

		respondWithJSON(w, http.StatusAccepted, DeleteJobResponse{
			Message: "deletion queued",
		})
	}
}

func containsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "not found")
}

func respondWithJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
