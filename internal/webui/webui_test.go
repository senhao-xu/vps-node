package webui

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

func TestWrapWithoutEmbedServesFallbackAndAPI(t *testing.T) {
	if Embedded() {
		t.Skip("built with embed_ui tag")
	}
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("api-hit"))
	})
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	handler := Wrap(api, logger)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if rec.Body.String() != "api-hit" {
		t.Fatalf("api passthrough failed: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Body.String() != "api-hit" {
		t.Fatalf("healthz passthrough failed: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Web UI not embedded") {
		t.Fatalf("fallback page missing: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users/1", nil))
	if rec.Body.String() != "api-hit" {
		t.Fatalf("non-root path without embed must fall through to api handler, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestWrapWithEmbedServesSubAndSPAFallback(t *testing.T) {
	apiCalled := ""
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = r.URL.Path
		if r.Method == http.MethodGet && r.URL.Path == "/s/token" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("subscription-body"))
			return
		}
		http.NotFound(w, r)
	})
	sub := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>spa-index</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log(1)")},
	}
	handler := wrapFS(sub, []byte("<html>spa-index</html>"), api)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/s/token", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "subscription-body" {
		t.Fatalf("subscription route must hit api, got %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("static asset must be served, got %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/users", nil))
	if apiCalled != "/users" {
		t.Fatalf("spa route must be offered to api first, api saw %q", apiCalled)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "spa-index") {
		t.Fatalf("api 404 must fall back to spa index, got %d %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("spa fallback content type = %q", ct)
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "spa-index") {
		t.Fatalf("api prefix must pass through without spa fallback, got %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/s/token", nil))
	if apiCalled != "/s/token" || rec.Code != http.StatusNotFound {
		t.Fatalf("non-GET unknown path must pass through to api, api saw %q, got %d", apiCalled, rec.Code)
	}
}

func TestWrapWithEmbedPreservesNon404APIStatus(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "subscription unavailable", http.StatusForbidden)
	})
	sub := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa-index</html>")},
	}
	handler := wrapFS(sub, []byte("<html>spa-index</html>"), api)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/s/expired", nil))
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "spa-index") {
		t.Fatalf("non-404 api status must pass through, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestWrapWithEmbedPreservesJSON404(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"subscription not found"}}`))
	})
	sub := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa-index</html>")},
	}
	handler := wrapFS(sub, []byte("<html>spa-index</html>"), api)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/s/badtoken", nil))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), "not_found") {
		t.Fatalf("json 404 must pass through, got %d %s", rec.Code, rec.Body.String())
	}
}
