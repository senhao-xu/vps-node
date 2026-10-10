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

	p := NewPublicIPProvider("", "", srv.URL, "")
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

	p := NewPublicIPProvider("", "", srv.URL, "")
	ctx := context.Background()
	if got := p.Get(ctx); got != "198.51.100.4" {
		t.Fatalf("Get()=%q", got)
	}
	p.mu.Lock()
	p.v4.fetchedAt = time.Now().Add(-2 * publicIPCacheTTL)
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

	p := NewPublicIPProvider("203.0.113.9", "", srv.URL, "")
	if got := p.Get(context.Background()); got != "203.0.113.9" {
		t.Fatalf("Get()=%q", got)
	}
}

func TestPublicIPv6Provider(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("2001:db8::7\n"))
	}))
	t.Cleanup(srv.Close)

	p := NewPublicIPProvider("", "", "", srv.URL)
	ctx := context.Background()
	if got := p.Get6(ctx); got != "2001:db8::7" {
		t.Fatalf("Get6()=%q", got)
	}
	if got := p.Get6(ctx); got != "2001:db8::7" || hits.Load() != 1 {
		t.Fatalf("Get6()=%q hits=%d want cached", got, hits.Load())
	}

	p.mu.Lock()
	p.v6.fetchedAt = time.Now().Add(-2 * publicIPCacheTTL)
	p.mu.Unlock()
	if got := p.Get6(ctx); got != "2001:db8::7" || hits.Load() != 2 {
		t.Fatalf("Get6()=%q hits=%d want refetch after TTL", got, hits.Load())
	}
}

func TestPublicIPv6StaticSkipsNetwork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("static ipv6 must not trigger requests")
	}))
	t.Cleanup(srv.Close)

	p := NewPublicIPProvider("", "2001:db8::9", "", srv.URL)
	if got := p.Get6(context.Background()); got != "2001:db8::9" {
		t.Fatalf("Get6()=%q", got)
	}
}

func TestPublicIPv6RejectsV4EndpointResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.7"))
	}))
	t.Cleanup(srv.Close)

	p := NewPublicIPProvider("", "", "", srv.URL)
	if got := p.Get6(context.Background()); got != "" {
		t.Fatalf("Get6()=%q want empty for v4 answer", got)
	}
}

func TestNormalizePublicIP(t *testing.T) {
	cases := map[string]struct {
		raw    string
		wantV6 bool
		want   string
	}{
		"v4 accepted":        {"203.0.113.7\n", false, "203.0.113.7"},
		"v4 trimmed":         {" 198.51.100.4 ", false, "198.51.100.4"},
		"v4 rejected for v6": {"203.0.113.7", true, ""},
		"v6 accepted":        {"2001:db8::1", true, "2001:db8::1"},
		"v6 rejected for v4": {"2001:db8::1", false, ""},
		"private v4":         {"10.0.0.1", false, ""},
		"loopback":           {"127.0.0.1", false, ""},
		"link local":         {"169.254.1.1", false, ""},
		"unspecified":        {"0.0.0.0", false, ""},
		"not an ip":          {"not-an-ip", false, ""},
		"empty":              {"", false, ""},
		"trailing junk":      {"2001:db8::1 extra", true, ""},
	}
	for name, tc := range cases {
		if got := normalizePublicIP(tc.raw, tc.wantV6); got != tc.want {
			t.Fatalf("%s: normalizePublicIP(%q,%v)=%q want %q", name, tc.raw, tc.wantV6, got, tc.want)
		}
	}
}
