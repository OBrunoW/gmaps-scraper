package httpadapter

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/OBrunoW/gmaps-scraper/internal/adapters/http/tools"
	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/middleware"
)

const requestTimeout = 120 * time.Second

func NewRouter(
	jobs *application.JobService,
	prospector *ProspectorHandlers,
	auth func(http.Handler) http.Handler,
) http.Handler {
	router := chi.NewRouter()
	router.Use(chimiddleware.Recoverer)
	Routes(router, jobs, prospector, auth)

	return router
}

func Routes(
	router chi.Router,
	jobs *application.JobService,
	prospector *ProspectorHandlers,
	auth func(http.Handler) http.Handler,
) {
	handlers := NewHandlers(jobs)

	router.Use(middleware.Logging)
	router.Use(chimiddleware.Recoverer)
	router.Use(chimiddleware.Timeout(requestTimeout))

	router.Get("/api/v1/health", handlers.health)
	router.Handle("/tools/*", tools.Handler())

	router.Group(func(protected chi.Router) {
		protected.Use(auth)
		protected.Post("/api/v1/scrape", handlers.createJob)
		protected.Get("/api/v1/jobs", handlers.listJobs)
		protected.Get("/api/v1/jobs/{job_id}", handlers.getJob)
		protected.Delete("/api/v1/jobs/{job_id}", handlers.deleteJob)
		prospectorRoutes(protected, prospector)
	})
}
