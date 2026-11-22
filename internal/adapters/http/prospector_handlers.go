package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/OBrunoW/gmaps-scraper/internal/application"
	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const (
	codeBadRequest    = "BAD_REQUEST"
	codeNotFound      = "NOT_FOUND"
	codeInternalError = "INTERNAL_ERROR"
)

type ProspectorService interface {
	Create(ctx context.Context, params domain.ProspectorParams) (application.CreatedProspectorJob, error)
	Get(ctx context.Context, publicID string) (domain.ProspectorJob, error)
}

type CEPLookup interface {
	Lookup(ctx context.Context, cep string) (domain.CEPLocation, error)
}

type CNPJLookup interface {
	Lookup(ctx context.Context, cnpj string) (domain.CNPJCompany, error)
}

type ProspectorHandlers struct {
	prospector ProspectorService
	ceps       CEPLookup
	cnpjs      CNPJLookup
}

func NewProspectorHandlers(prospector ProspectorService, ceps CEPLookup, cnpjs CNPJLookup) *ProspectorHandlers {
	return &ProspectorHandlers{prospector: prospector, ceps: ceps, cnpjs: cnpjs}
}

func (h *ProspectorHandlers) createJob(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var request ProspectorJobRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeProspectorError(w, domain.InvalidInput("invalid request body"))
		return
	}

	created, err := h.prospector.Create(r.Context(), request.toParams())
	if err != nil {
		writeProspectorError(w, err)
		return
	}

	writeJSON(w, http.StatusAccepted, ProspectorJobCreatedResponse{
		JobID:    created.PublicID,
		Status:   string(created.Status),
		Keyword:  created.Keyword,
		City:     created.City,
		RadiusKm: created.RadiusKm,
	})
}

func (h *ProspectorHandlers) getJob(w http.ResponseWriter, r *http.Request) {
	publicID := chi.URLParam(r, "job_id")

	job, err := h.prospector.Get(r.Context(), publicID)
	if err != nil {
		writeProspectorError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newProspectorJobStatusResponse(publicID, job))
}

func (h *ProspectorHandlers) getCEP(w http.ResponseWriter, r *http.Request) {
	if h.ceps == nil {
		writeProspectorError(w, domain.ServiceNotConfigured())
		return
	}

	location, err := h.ceps.Lookup(r.Context(), chi.URLParam(r, "cep"))
	if err != nil {
		writeProspectorError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newCEPResponse(location))
}

func (h *ProspectorHandlers) getCNPJ(w http.ResponseWriter, r *http.Request) {
	if h.cnpjs == nil {
		writeProspectorError(w, domain.ServiceNotConfigured())
		return
	}

	company, err := h.cnpjs.Lookup(r.Context(), chi.URLParam(r, "cnpj"))
	if err != nil {
		writeProspectorError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, newCNPJResponse(company))
}

func writeProspectorError(w http.ResponseWriter, err error) {
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		writeJSON(w, appErr.Status, ProspectorErrorResponse{
			Message: appErr.Message,
			Code:    prospectorErrorCode(appErr.Code),
		})
		return
	}

	log.Error("unexpected error", "error", err)
	writeJSON(w, http.StatusInternalServerError, ProspectorErrorResponse{
		Message: internalErrorMessage,
		Code:    codeInternalError,
	})
}

func prospectorErrorCode(code string) string {
	switch code {
	case domain.CodeInvalidInput:
		return codeBadRequest
	case domain.CodeNotFound:
		return codeNotFound
	case domain.CodeInternal:
		return codeInternalError
	default:
		return code
	}
}
