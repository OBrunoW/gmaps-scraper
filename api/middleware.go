package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

type contextKey string

const callerContextKey contextKey = "caller"

type CallerInfo struct {
	Name string
}

type AuthConfig struct {
	ServiceKeys map[string]string
	APIKey      string
	AllowNoop   bool
}

func AuthFromEnv() AuthConfig {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	if appEnv == "" {
		appEnv = "development"
	}

	keys := parseServiceKeys(os.Getenv("SERVICE_KEYS"))
	apiKey := strings.TrimSpace(os.Getenv("API_KEY"))

	cfg := AuthConfig{
		ServiceKeys: keys,
		APIKey:      apiKey,
		AllowNoop:   appEnv == "development" && len(keys) == 0 && apiKey == "",
	}

	if appEnv != "development" && len(keys) == 0 && apiKey == "" {
		panic("SERVICE_KEYS or API_KEY is required outside development")
	}

	return cfg
}

func parseServiceKeys(raw string) map[string]string {
	out := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		secret := strings.TrimSpace(parts[1])
		if name != "" && secret != "" {
			out[secret] = name
		}
	}
	return out
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
	a := []byte(expected)
	b := []byte(provided)
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare(a, b) == 1
}

func KeyAuth(cfg AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.AllowNoop {
				ctx := context.WithValue(r.Context(), callerContextKey, &CallerInfo{Name: "dev"})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if svc := strings.TrimSpace(r.Header.Get("X-Service-Key")); svc != "" {
				name, ok := findServiceByKey(cfg.ServiceKeys, svc)
				if !ok {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
				ctx := context.WithValue(r.Context(), callerContextKey, &CallerInfo{Name: name})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			apiKey := strings.TrimSpace(r.Header.Get("X-Api-Key"))
			if apiKey == "" {
				apiKey = strings.TrimSpace(r.Header.Get("X-API-Key"))
			}
			if apiKey == "" {
				auth := r.Header.Get("Authorization")
				trimmed := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
				if trimmed != auth {
					apiKey = trimmed
				}
			}

			if matchAPIKey(cfg.APIKey, apiKey) {
				ctx := context.WithValue(r.Context(), callerContextKey, &CallerInfo{Name: "api"})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		})
	}
}

func CallerFromContext(ctx context.Context) *CallerInfo {
	if c, ok := ctx.Value(callerContextKey).(*CallerInfo); ok {
		return c
	}
	return nil
}
