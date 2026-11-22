package httpadapter

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
)

func TestWriteErrorWithAppError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, domain.NotFound("job not found"))
	assertJSON(t, recorder, http.StatusNotFound, `{"message":"job not found"}`)
}

func TestWriteErrorUsesMessageNotCause(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, domain.Internal("failed to create job", errors.New("secret detail")))
	assertJSON(t, recorder, http.StatusInternalServerError, `{"message":"failed to create job"}`)
}

func TestWriteErrorUnwrapsWrappedAppError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, fmt.Errorf("wrapped: %w", domain.InvalidInput("bad")))
	assertJSON(t, recorder, http.StatusBadRequest, `{"message":"bad"}`)
}

func TestWriteErrorUntypedBecomesInternal(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, errors.New("unexpected"))
	assertJSON(t, recorder, http.StatusInternalServerError, `{"message":"internal error"}`)
}
