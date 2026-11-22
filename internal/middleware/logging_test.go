package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoggingUsesCFRayAsRequestID(t *testing.T) {
	var seen string
	handler := Logging(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusTeapot)
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("CF-Ray", "ray-1")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if seen != "ray-1" || recorder.Code != http.StatusTeapot {
		t.Fatalf("request id %q status %d", seen, recorder.Code)
	}
}

func TestLoggingGeneratesRequestID(t *testing.T) {
	var seen string
	handler := Logging(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = RequestIDFromContext(r.Context())
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if len(seen) != 32 {
		t.Fatalf("request id = %q", seen)
	}
}

func TestRequestIDFromContextEmpty(t *testing.T) {
	if got := RequestIDFromContext(httptest.NewRequest(http.MethodGet, "/", nil).Context()); got != "" {
		t.Fatalf("got %q", got)
	}
}
