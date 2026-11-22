package domain

import (
	"errors"
	"net/http"
	"testing"
)

func TestAppErrorConstructors(t *testing.T) {
	cause := errors.New("boom")

	tests := []struct {
		name       string
		err        *AppError
		wantStatus int
		wantCode   string
		wantText   string
	}{
		{"invalid input", InvalidInput("bad"), http.StatusBadRequest, CodeInvalidInput, "bad"},
		{"not found", NotFound("missing"), http.StatusNotFound, CodeNotFound, "missing"},
		{"internal", Internal("failed", cause), http.StatusInternalServerError, CodeInternal, "failed: boom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Status != tt.wantStatus {
				t.Errorf("status = %d, want %d", tt.err.Status, tt.wantStatus)
			}
			if tt.err.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", tt.err.Code, tt.wantCode)
			}
			if tt.err.Error() != tt.wantText {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.wantText)
			}
		})
	}
}

func TestAppErrorUnwrap(t *testing.T) {
	cause := errors.New("boom")
	err := error(Internal("failed", cause))

	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the wrapped cause")
	}

	var appErr *AppError
	if !errors.As(err, &appErr) {
		t.Fatal("errors.As should find AppError")
	}
	if appErr.Message != "failed" {
		t.Errorf("message = %q, want failed", appErr.Message)
	}
	if InvalidInput("x").Unwrap() != nil {
		t.Error("Unwrap without cause should be nil")
	}
}
