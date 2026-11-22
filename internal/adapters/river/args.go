package river

import (
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/riverqueue/river"
)

const (
	KindScrape    = "scrape"
	KindJobDelete = "job_delete"
	KindCleanup   = "cleanup"

	QueueMaintenance = "maintenance"
)

type ScrapeJobArgs struct {
	Keyword        string  `json:"keyword"`
	Lang           string  `json:"lang"`
	MaxDepth       int     `json:"max_depth"`
	Email          bool    `json:"email"`
	GeoCoordinates string  `json:"geo_coordinates"`
	Zoom           int     `json:"zoom"`
	Radius         float64 `json:"radius"`
	FastMode       bool    `json:"fast_mode"`
	ExtraReviews   bool    `json:"extra_reviews"`
	TimeoutSecs    int     `json:"timeout"`
}

func (ScrapeJobArgs) Kind() string {
	return KindScrape
}

func (ScrapeJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

func (a ScrapeJobArgs) params() domain.ScrapeParams {
	return domain.ScrapeParams{
		Keyword:        a.Keyword,
		Lang:           a.Lang,
		MaxDepth:       a.MaxDepth,
		Email:          a.Email,
		GeoCoordinates: a.GeoCoordinates,
		Zoom:           a.Zoom,
		Radius:         a.Radius,
		FastMode:       a.FastMode,
		ExtraReviews:   a.ExtraReviews,
		TimeoutSecs:    a.TimeoutSecs,
	}
}

func argsFromParams(params domain.ScrapeParams) ScrapeJobArgs {
	return ScrapeJobArgs{
		Keyword:        params.Keyword,
		Lang:           params.Lang,
		MaxDepth:       params.MaxDepth,
		Email:          params.Email,
		GeoCoordinates: params.GeoCoordinates,
		Zoom:           params.Zoom,
		Radius:         params.Radius,
		FastMode:       params.FastMode,
		ExtraReviews:   params.ExtraReviews,
		TimeoutSecs:    params.TimeoutSecs,
	}
}

type JobDeleteArgs struct {
	JobID int64 `json:"job_id"`
}

func (JobDeleteArgs) Kind() string {
	return KindJobDelete
}

func (JobDeleteArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueMaintenance, MaxAttempts: 3}
}

type CleanupArgs struct{}

func (CleanupArgs) Kind() string {
	return KindCleanup
}

func (CleanupArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueMaintenance, MaxAttempts: 1}
}
