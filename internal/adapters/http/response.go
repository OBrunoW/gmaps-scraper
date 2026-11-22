package httpadapter

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/log"
)

const internalErrorMessage = "internal error"

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error) {
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		writeJSON(w, appErr.Status, ErrorResponse{Message: appErr.Message})
		return
	}

	log.Error("unexpected error", "error", err)
	writeJSON(w, http.StatusInternalServerError, ErrorResponse{Message: internalErrorMessage})
}
