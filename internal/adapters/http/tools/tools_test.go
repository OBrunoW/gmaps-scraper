package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}

func TestIndexListsPages(t *testing.T) {
	recorder := serve("/tools/")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	for _, page := range []string{"scrape.html", "prospector.html", "lead-learning.html"} {
		if !strings.Contains(recorder.Body.String(), `href="`+page+`"`) {
			t.Errorf("index missing link to %s", page)
		}
	}
}

func TestPagesServedAsHTML(t *testing.T) {
	for _, page := range []string{"scrape.html", "prospector.html", "lead-learning.html"} {
		recorder := serve("/tools/" + page)
		if recorder.Code != http.StatusOK {
			t.Errorf("%s status = %d", page, recorder.Code)
		}
		if !strings.HasPrefix(recorder.Header().Get("Content-Type"), "text/html") {
			t.Errorf("%s content type = %q", page, recorder.Header().Get("Content-Type"))
		}
	}
}

func TestUnknownPathNotFound(t *testing.T) {
	if recorder := serve("/tools/missing.html"); recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}
