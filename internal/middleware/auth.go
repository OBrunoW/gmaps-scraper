package middleware

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

type contextKey string

const callerContextKey contextKey = "caller"

const (
	serviceKeyHeader  = "X-Service-Key"
	serviceNameHeader = "X-Service-Name"
	unauthorizedBody  = `{"error":"unauthorized"}`
	devCallerName     = "dev"
	apiCallerName     = "api"
)

type ServiceAuth struct {
	keys   map[string]string
	apiKey string
}

func NewServiceAuth(keys map[string]string, apiKey string) *ServiceAuth {
	return &ServiceAuth{keys: keys, apiKey: apiKey}
}

func (a *ServiceAuth) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if provided := strings.TrimSpace(r.Header.Get(serviceKeyHeader)); provided != "" {
			name, ok := findServiceByKey(a.keys, provided)
			if !ok {
				http.Error(w, unauthorizedBody, http.StatusUnauthorized)
				return
			}
			serveAs(next, w, r, name)
			return
		}

		if matchAPIKey(a.apiKey, extractAPIKey(r)) {
			serveAs(next, w, r, apiCallerName)
			return
		}

		http.Error(w, unauthorizedBody, http.StatusUnauthorized)
	})
}

func NoopAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveAs(next, w, r, devCallerName)
	})
}

func CallerName(ctx context.Context) string {
	name, _ := ctx.Value(callerContextKey).(string)
	return name
}

func serveAs(next http.Handler, w http.ResponseWriter, r *http.Request, name string) {
	r.Header.Set(serviceNameHeader, name)
	ctx := context.WithValue(r.Context(), callerContextKey, name)
	next.ServeHTTP(w, r.WithContext(ctx))
}

func extractAPIKey(r *http.Request) string {
	if apiKey := strings.TrimSpace(r.Header.Get("X-Api-Key")); apiKey != "" {
		return apiKey
	}

	if apiKey := strings.TrimSpace(r.Header.Get("X-API-Key")); apiKey != "" {
		return apiKey
	}

	authorization := r.Header.Get("Authorization")
	token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	if token != authorization {
		return token
	}

	return ""
}

func findServiceByKey(keys map[string]string, provided string) (string, bool) {
	providedBytes := []byte(provided)
	var matched string
	found := false
	for secret, name := range keys {
		secretBytes := []byte(secret)
		if len(secretBytes) != len(providedBytes) {
			continue
		}
		if subtle.ConstantTimeCompare(secretBytes, providedBytes) == 1 {
			matched = name
			found = true
		}
	}
	return matched, found
}

func matchAPIKey(expected, provided string) bool {
	if expected == "" || provided == "" {
		return false
	}
	expectedBytes := []byte(expected)
	providedBytes := []byte(provided)
	if len(expectedBytes) != len(providedBytes) {
		return false
	}
	return subtle.ConstantTimeCompare(expectedBytes, providedBytes) == 1
}
