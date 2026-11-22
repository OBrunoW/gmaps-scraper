package httpadapter

import (
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
)

type ErrorResponse struct {
	Message string `json:"message" example:"invalid request body"`
}

type HealthCheckResponse struct {
	Status string `json:"status" example:"ok"`
}

type ScrapeRequest struct {
	Keyword        string  `json:"keyword" example:"restaurants in New York"`
	Lang           string  `json:"lang,omitempty" example:"en"`
	MaxDepth       int     `json:"max_depth,omitempty" example:"1"`
	Email          bool    `json:"email,omitempty" example:"false"`
	GeoCoordinates string  `json:"geo_coordinates,omitempty" example:"40.7128,-74.0060"`
	Zoom           int     `json:"zoom,omitempty" example:"14"`
	Radius         float64 `json:"radius,omitempty" example:"5.0"`
	FastMode       bool    `json:"fast_mode,omitempty" example:"false"`
	ExtraReviews   bool    `json:"extra_reviews,omitempty" example:"false"`
	Timeout        int     `json:"timeout,omitempty" example:"300"`
}

func (r ScrapeRequest) toParams() domain.ScrapeParams {
	return domain.ScrapeParams{
		Keyword:        r.Keyword,
		Lang:           r.Lang,
		MaxDepth:       r.MaxDepth,
		Email:          r.Email,
		GeoCoordinates: r.GeoCoordinates,
		Zoom:           r.Zoom,
		Radius:         r.Radius,
		FastMode:       r.FastMode,
		ExtraReviews:   r.ExtraReviews,
		TimeoutSecs:    r.Timeout,
	}
}

type ScrapeResponse struct {
	JobID  string `json:"job_id" example:"kYzR8xLmNpQvWjX3"`
	Status string `json:"status" example:"pending"`
}

type JobSummary struct {
	JobID       string     `json:"job_id" example:"kYzR8xLmNpQvWjX3"`
	Status      string     `json:"status" example:"completed"`
	Keyword     string     `json:"keyword" example:"restaurants in New York"`
	CreatedAt   time.Time  `json:"created_at" example:"2024-01-15T10:30:00Z"`
	StartedAt   *time.Time `json:"started_at,omitempty" example:"2024-01-15T10:30:05Z"`
	CompletedAt *time.Time `json:"completed_at,omitempty" example:"2024-01-15T10:35:00Z"`
	ResultCount int        `json:"result_count" example:"25"`
	Error       string     `json:"error,omitempty" example:""`
}

type ListJobsResponse struct {
	Jobs       []JobSummary `json:"jobs"`
	NextCursor string       `json:"next_cursor,omitempty" example:"eyJpZCI6MTIzfQ=="`
	HasMore    bool         `json:"has_more" example:"true"`
}

type JobStatusResponse struct {
	JobID       string     `json:"job_id" example:"kYzR8xLmNpQvWjX3"`
	Status      string     `json:"status" example:"completed"`
	Keyword     string     `json:"keyword" example:"restaurants in New York"`
	CreatedAt   time.Time  `json:"created_at" example:"2024-01-15T10:30:00Z"`
	StartedAt   *time.Time `json:"started_at,omitempty" example:"2024-01-15T10:30:05Z"`
	CompletedAt *time.Time `json:"completed_at,omitempty" example:"2024-01-15T10:35:00Z"`
	Results     any        `json:"results,omitempty"`
	Error       string     `json:"error,omitempty" example:""`
	ResultCount int        `json:"result_count" example:"25"`
}

type DeleteJobResponse struct {
	Message string `json:"message" example:"deletion queued"`
}
