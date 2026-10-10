package agentruntime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublicIPProviderFetchesAndCaches(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("203.0.113.7\n"))
	}))
	t.Cleanup(srv.Close)

	p := NewPublicIPProvider("", srv.URL)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if got := p.Get(ctx); got != "203.0.113.7" {
			t.Fatalf("Get()=%q want 203.0.113.7", got)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("expected single fetch within TTL, got %d", hits.Load())
	}
}

func TestPublicIPProviderStaleKeptOnFailure(t *testing.T) {
	var ok atomic.Bool
	ok.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ok.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("198.51.100.4"))
	}))
	t.Cleanup(srv.Close)

	p := NewPublicIPProvider("", srv.URL)
	ctx := context.Background()
	if got := p.Get(ctx); got != "198.51.100.4" {
		t.Fatalf("Get()=%q", got)
	}
	p.mu.Lock()
	p.fetchedAt = time.Now().Add(-2 * publicIPCacheTTL)
	p.mu.Unlock()
	ok.Store(false)
	if got := p.Get(ctx); got != "198.51.100.4" {
		t.Fatalf("stale cache must survive fetch failure, got %q", got)
	}
}

func TestPublicIPProviderStaticSkipsNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("static ip must not trigger requests")
	}))
	t.Cleanup(srv.Close)

	p := NewPublicIPProvider("203.0.113.9", srv.URL)
	if got := p.Get(context.Background()); got != "203.0.113.9" {
		t.Fatalf("Get()=%q", got)
	}
}

func TestNormalizePublicIP(t *testing.T) {
	cases := map[string]string{
		"203.0.113.7\n":     "203.0.113.7",
		" 198.51.100.4 ":    "198.51.100.4",
		"2001:db8::1":       "2001:db8::1",
		"10.0.0.1":          "",
		"127.0.0.1":         "",
		"169.254.1.1":       "",
		"0.0.0.0":           "",
		"not-an-ip":         "",
		"":                  "",
		"2001:db8::1 extra": "",
	}
	for raw, want := range cases {
		if got := normalizePublicIP(raw); got != want {
			t.Fatalf("normalizePublicIP(%q)=%q want %q", raw, got, want)
		}
	}
}
