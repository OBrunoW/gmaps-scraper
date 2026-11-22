package httpadapter

import "github.com/go-chi/chi/v5"

func prospectorRoutes(router chi.Router, handlers *ProspectorHandlers) {
	router.Post("/api/v1/prospector/jobs", handlers.createJob)
	router.Get("/api/v1/prospector/jobs/{job_id}", handlers.getJob)
	router.Get("/api/v1/prospector/cep/{cep}", handlers.getCEP)
	router.Get("/api/v1/prospector/cnpj/{cnpj}", handlers.getCNPJ)
}
