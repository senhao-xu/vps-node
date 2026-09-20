package webui

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
