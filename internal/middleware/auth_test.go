package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestAuth() *ServiceAuth {
	return NewServiceAuth(map[string]string{"crm-secret": "crm"}, "legacy-key")
}

type recordedCall struct {
	called  bool
	caller  string
	service string
}

func serve(t *testing.T, handler func(http.Handler) http.Handler, headers map[string]string) (*httptest.ResponseRecorder, *recordedCall) {
	t.Helper()
	recorded := &recordedCall{}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorded.called = true
		recorded.caller = CallerName(r.Context())
		recorded.service = r.Header.Get("X-Service-Name")
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	handler(next).ServeHTTP(response, request)
	return response, recorded
}

func assertUnauthorized(t *testing.T, response *httptest.ResponseRecorder, recorded *recordedCall) {
	t.Helper()
	if recorded.called {
		t.Error("next handler must not be called")
	}
	if response.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", response.Code)
	}
	if body := response.Body.String(); body != "{\"error\":\"unauthorized\"}\n" {
		t.Errorf("body = %q", body)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
		t.Errorf("content type = %q", contentType)
	}
}

func TestServiceAuthValidServiceKey(t *testing.T) {
	response, recorded := serve(t, newTestAuth().Handler, map[string]string{"X-Service-Key": " crm-secret "})

	if response.Code != http.StatusNoContent || !recorded.called {
		t.Fatalf("status = %d, called = %v", response.Code, recorded.called)
	}
	if recorded.caller != "crm" || recorded.service != "crm" {
		t.Errorf("caller = %q, service header = %q, want crm", recorded.caller, recorded.service)
	}
}

func TestServiceAuthInvalidServiceKeyDoesNotFallBack(t *testing.T) {
	response, recorded := serve(t, newTestAuth().Handler, map[string]string{
		"X-Service-Key": "wrong",
		"X-Api-Key":     "legacy-key",
	})

	assertUnauthorized(t, response, recorded)
}

func TestServiceAuthAPIKeyHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
	}{
		{"x-api-key", map[string]string{"X-Api-Key": "legacy-key"}},
		{"x-api-key trimmed", map[string]string{"X-Api-Key": "  legacy-key  "}},
		{"bearer", map[string]string{"Authorization": "Bearer legacy-key"}},
		{"bearer trimmed", map[string]string{"Authorization": "Bearer   legacy-key  "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, recorded := serve(t, newTestAuth().Handler, tt.headers)

			if response.Code != http.StatusNoContent || !recorded.called {
				t.Fatalf("status = %d, called = %v", response.Code, recorded.called)
			}
			if recorded.caller != "api" || recorded.service != "api" {
				t.Errorf("caller = %q, service header = %q, want api", recorded.caller, recorded.service)
			}
		})
	}
}

func TestServiceAuthRejects(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
	}{
		{"no credentials", nil},
		{"wrong api key", map[string]string{"X-Api-Key": "nope"}},
		{"wrong bearer", map[string]string{"Authorization": "Bearer nope"}},
		{"non bearer scheme", map[string]string{"Authorization": "legacy-key"}},
		{"basic scheme", map[string]string{"Authorization": "Basic legacy-key"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, recorded := serve(t, newTestAuth().Handler, tt.headers)
			assertUnauthorized(t, response, recorded)
		})
	}
}

func TestServiceAuthWithoutConfiguredKeysRejectsEverything(t *testing.T) {
	handler := NewServiceAuth(nil, "").Handler
	response, recorded := serve(t, handler, map[string]string{"X-Api-Key": "anything"})

	assertUnauthorized(t, response, recorded)
}

func TestNoopAuth(t *testing.T) {
	response, recorded := serve(t, NoopAuth, nil)

	if response.Code != http.StatusNoContent || !recorded.called {
		t.Fatalf("status = %d, called = %v", response.Code, recorded.called)
	}
	if recorded.caller != "dev" || recorded.service != "dev" {
		t.Errorf("caller = %q, service header = %q, want dev", recorded.caller, recorded.service)
	}
}

func TestCallerNameWithoutIdentity(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if name := CallerName(request.Context()); name != "" {
		t.Errorf("caller = %q, want empty", name)
	}
}
