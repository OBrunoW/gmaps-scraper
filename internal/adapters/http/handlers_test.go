package httpadapter

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OBrunoW/gmaps-scraper/internal/domain"
	"github.com/OBrunoW/gmaps-scraper/internal/middleware"
)

func do(env *testEnv, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	env.router.ServeHTTP(recorder, request)
	return recorder
}

func assertJSON(t *testing.T, recorder *httptest.ResponseRecorder, status int, body string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d, body %q", recorder.Code, status, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("content type = %q", contentType)
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != body {
		t.Fatalf("body = %s, want %s", got, body)
	}
}

func TestHealthIsPublic(t *testing.T) {
	env := newTestEnv(middleware.NewServiceAuth(map[string]string{"k": "svc"}, "").Handler)
	assertJSON(t, do(env, http.MethodGet, "/api/v1/health", "", nil), 200, `{"status":"ok"}`)
}

func TestProtectedRoutesReturnUnauthorized(t *testing.T) {
	env := newTestEnv(middleware.NewServiceAuth(map[string]string{"k": "svc"}, "").Handler)
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/scrape"},
		{http.MethodGet, "/api/v1/jobs"},
		{http.MethodGet, "/api/v1/jobs/pub1"},
		{http.MethodDelete, "/api/v1/jobs/pub1"},
	}
	for _, route := range routes {
		recorder := do(env, route.method, route.path, "", map[string]string{"X-Service-Key": "wrong"})
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d", route.method, route.path, recorder.Code)
		}
		if got := recorder.Body.String(); got != "{\"error\":\"unauthorized\"}\n" {
			t.Fatalf("body = %q", got)
		}
		if contentType := recorder.Header().Get("Content-Type"); contentType != "text/plain; charset=utf-8" {
			t.Fatalf("content type = %q", contentType)
		}
	}
}

func TestProtectedRouteWithValidKey(t *testing.T) {
	env := newTestEnv(middleware.NewServiceAuth(map[string]string{"k": "svc"}, "").Handler)
	recorder := do(env, http.MethodGet, "/api/v1/jobs", "", map[string]string{"X-Service-Key": "k"})
	assertJSON(t, recorder, 200, `{"jobs":[],"has_more":false}`)
}

func TestCreateJobAccepted(t *testing.T) {
	env := newTestEnv(nil)
	recorder := do(env, http.MethodPost, "/api/v1/scrape", `{"keyword":"pizza","lang":"pt","max_depth":2,"timeout":60}`, nil)
	assertJSON(t, recorder, 202, `{"job_id":"pub7","status":"pending"}`)

	want := domain.ScrapeParams{Keyword: "pizza", Lang: "pt", MaxDepth: 2, TimeoutSecs: 60}
	if len(env.queue.enqueued) != 1 || env.queue.enqueued[0] != want {
		t.Fatalf("enqueued = %+v", env.queue.enqueued)
	}
}

func TestCreateJobAppliesDefaults(t *testing.T) {
	env := newTestEnv(nil)
	do(env, http.MethodPost, "/api/v1/scrape", `{"keyword":"pizza"}`, nil)
	want := domain.ScrapeParams{Keyword: "pizza", Lang: "en", MaxDepth: 1, TimeoutSecs: 300}
	if env.queue.enqueued[0] != want {
		t.Fatalf("enqueued = %+v", env.queue.enqueued[0])
	}
}

func TestCreateJobErrors(t *testing.T) {
	oversized := `{"keyword":"` + strings.Repeat("a", 1<<20) + `"}`
	tests := []struct {
		name   string
		body   string
		queue  error
		status int
		want   string
	}{
		{"invalid json", `{not json`, nil, 400, `{"message":"invalid request body"}`},
		{"empty body", ``, nil, 400, `{"message":"invalid request body"}`},
		{"oversized body", oversized, nil, 400, `{"message":"invalid request body"}`},
		{"missing keyword", `{}`, nil, 400, `{"message":"keyword is required"}`},
		{"bad coordinates", `{"keyword":"a","geo_coordinates":"1,2,3"}`, nil, 400, `{"message":"geo_coordinates must contain exactly one comma"}`},
		{"timeout out of range", `{"keyword":"a","timeout":301}`, nil, 400, `{"message":"timeout must be between 1 and 300 seconds"}`},
		{"queue failure", `{"keyword":"a"}`, errors.New("db down"), 500, `{"message":"failed to create job"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnv(nil)
			env.queue.enqueueErr = test.queue
			assertJSON(t, do(env, http.MethodPost, "/api/v1/scrape", test.body, nil), test.status, test.want)
		})
	}
}

func TestListJobsPayload(t *testing.T) {
	env := newTestEnv(nil)
	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	started := created.Add(5 * time.Second)
	finished := created.Add(5 * time.Minute)
	env.queue.page = domain.JobPage{
		Records: []domain.JobRecord{
			{ID: 1, State: domain.StateCompleted, Keyword: "pizza", CreatedAt: created, AttemptedAt: &started, FinalizedAt: &finished},
			{ID: 2, State: domain.StateDiscarded, Keyword: "sushi", CreatedAt: created, LastError: "boom"},
		},
		NextCursor: "next",
	}
	env.results.counts = map[int64]int{1: 25}

	recorder := do(env, http.MethodGet, "/api/v1/jobs?limit=2&state=completed", "", nil)
	want := `{"jobs":[` +
		`{"job_id":"pub1","status":"completed","keyword":"pizza","created_at":"2024-01-15T10:30:00Z","started_at":"2024-01-15T10:30:05Z","completed_at":"2024-01-15T10:35:00Z","result_count":25},` +
		`{"job_id":"pub2","status":"failed","keyword":"sushi","created_at":"2024-01-15T10:30:00Z","result_count":0,"error":"boom"}` +
		`],"next_cursor":"next","has_more":true}`
	assertJSON(t, recorder, 200, want)
}

func TestListJobsEmptyIsArray(t *testing.T) {
	env := newTestEnv(nil)
	assertJSON(t, do(env, http.MethodGet, "/api/v1/jobs", "", nil), 200, `{"jobs":[],"has_more":false}`)
}

func TestListJobsInvalidLimitBecomesDefault(t *testing.T) {
	for _, limit := range []string{"abc", "0", "-5", ""} {
		env := newTestEnv(nil)
		target := "/api/v1/jobs"
		if limit != "" {
			target += "?limit=" + limit
		}
		do(env, http.MethodGet, target, "", nil)
		if env.queue.listFilter.Limit != 20 {
			t.Fatalf("limit %q produced %d", limit, env.queue.listFilter.Limit)
		}
	}
}

func TestListJobsLimitClampedByService(t *testing.T) {
	env := newTestEnv(nil)
	do(env, http.MethodGet, "/api/v1/jobs?limit=500", "", nil)
	if env.queue.listFilter.Limit != 100 {
		t.Fatalf("limit = %d", env.queue.listFilter.Limit)
	}
}

func TestListJobsErrors(t *testing.T) {
	env := newTestEnv(nil)
	assertJSON(t, do(env, http.MethodGet, "/api/v1/jobs?state=bogus", "", nil), 400, `{"message":"invalid state: bogus"}`)

	env = newTestEnv(nil)
	env.queue.listErr = errors.New("connection refused")
	assertJSON(t, do(env, http.MethodGet, "/api/v1/jobs", "", nil), 400, `{"message":"connection refused"}`)

	env = newTestEnv(nil)
	env.queue.listErr = domain.InvalidInput("invalid cursor: bad")
	assertJSON(t, do(env, http.MethodGet, "/api/v1/jobs?cursor=x", "", nil), 400, `{"message":"invalid cursor: bad"}`)
}

func TestGetJobCompletedDecodesResults(t *testing.T) {
	env := newTestEnv(nil)
	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	finished := created.Add(time.Minute)
	env.queue.records[3] = domain.JobRecord{ID: 3, State: domain.StateCompleted, Keyword: "pizza", CreatedAt: created, AttemptedAt: &created, FinalizedAt: &finished}
	env.results.result = domain.ScrapeResult{JobID: 3, Results: json.RawMessage(`[{"title":"A"},{"title":"B"}]`), ResultCount: 2}

	recorder := do(env, http.MethodGet, "/api/v1/jobs/pub3", "", nil)
	want := `{"job_id":"pub3","status":"completed","keyword":"pizza","created_at":"2024-01-15T10:30:00Z","started_at":"2024-01-15T10:30:00Z","completed_at":"2024-01-15T10:31:00Z","results":[{"title":"A"},{"title":"B"}],"result_count":2}`
	assertJSON(t, recorder, 200, want)
}

func TestGetJobResultsDecodedAsAny(t *testing.T) {
	env := newTestEnv(nil)
	env.queue.records[3] = domain.JobRecord{ID: 3, State: domain.StateCompleted, CreatedAt: time.Now()}
	env.results.result = domain.ScrapeResult{Results: json.RawMessage(`{"nested":{"n":1}}`), ResultCount: 1}

	recorder := do(env, http.MethodGet, "/api/v1/jobs/pub3", "", nil)
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	results, ok := payload["results"].(map[string]any)
	if !ok || results["nested"] == nil {
		t.Fatalf("results = %#v", payload["results"])
	}
}

func TestGetJobStatuses(t *testing.T) {
	created := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name   string
		record domain.JobRecord
		want   string
	}{
		{"pending state", domain.JobRecord{ID: 4, State: domain.StatePending, Keyword: "k", CreatedAt: created},
			`{"job_id":"pub4","status":"pending","keyword":"k","created_at":"2024-01-15T10:30:00Z","result_count":0}`},
		{"available state", domain.JobRecord{ID: 4, State: domain.StateAvailable, Keyword: "k", CreatedAt: created},
			`{"job_id":"pub4","status":"pending","keyword":"k","created_at":"2024-01-15T10:30:00Z","result_count":0}`},
		{"running state", domain.JobRecord{ID: 4, State: domain.StateRunning, Keyword: "k", CreatedAt: created, AttemptedAt: &created},
			`{"job_id":"pub4","status":"running","keyword":"k","created_at":"2024-01-15T10:30:00Z","started_at":"2024-01-15T10:30:00Z","result_count":0}`},
		{"discarded state", domain.JobRecord{ID: 4, State: domain.StateDiscarded, Keyword: "k", CreatedAt: created, LastError: "bad"},
			`{"job_id":"pub4","status":"failed","keyword":"k","created_at":"2024-01-15T10:30:00Z","error":"bad","result_count":0}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env := newTestEnv(nil)
			env.queue.records[4] = test.record
			assertJSON(t, do(env, http.MethodGet, "/api/v1/jobs/pub4", "", nil), 200, test.want)
		})
	}
}

func TestGetJobNotFound(t *testing.T) {
	env := newTestEnv(nil)
	assertJSON(t, do(env, http.MethodGet, "/api/v1/jobs/pub99", "", nil), 404, `{"message":"job not found"}`)
	assertJSON(t, do(env, http.MethodGet, "/api/v1/jobs/garbage", "", nil), 404, `{"message":"job not found"}`)
}

func TestDeleteJobAccepted(t *testing.T) {
	env := newTestEnv(nil)
	env.queue.records[5] = domain.JobRecord{ID: 5, State: domain.StateRunning}
	assertJSON(t, do(env, http.MethodDelete, "/api/v1/jobs/pub5", "", nil), 202, `{"message":"deletion queued"}`)
	if len(env.queue.purgedIDs) != 1 || env.queue.purgedIDs[0] != 5 {
		t.Fatalf("purged = %v", env.queue.purgedIDs)
	}
	if len(env.queue.cancelledIDs) != 1 {
		t.Fatalf("cancelled = %v", env.queue.cancelledIDs)
	}
}

func TestDeleteJobErrors(t *testing.T) {
	env := newTestEnv(nil)
	assertJSON(t, do(env, http.MethodDelete, "/api/v1/jobs/pub99", "", nil), 404, `{"message":"job not found"}`)

	recorder := do(env, http.MethodDelete, "/api/v1/jobs/garbage", "", nil)
	if recorder.Code != 500 || !strings.HasPrefix(recorder.Body.String(), `{"message":"invalid job id: `) {
		t.Fatalf("status %d body %q", recorder.Code, recorder.Body.String())
	}

	env = newTestEnv(nil)
	env.queue.records[5] = domain.JobRecord{ID: 5, State: domain.StateAvailable}
	env.queue.purgeErr = errors.New("insert failed")
	assertJSON(t, do(env, http.MethodDelete, "/api/v1/jobs/pub5", "", nil), 500, `{"message":"failed to queue deletion: insert failed"}`)
}

func TestUnknownRouteIsChiNotFound(t *testing.T) {
	env := newTestEnv(nil)
	recorder := do(env, http.MethodGet, "/api/v1/unknown", "", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
	recorder = do(env, http.MethodPut, "/api/v1/health", "", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestPanicIsRecovered(t *testing.T) {
	env := newTestEnv(func(http.Handler) http.Handler {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	})
	recorder := do(env, http.MethodGet, "/api/v1/jobs", "", nil)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
}
