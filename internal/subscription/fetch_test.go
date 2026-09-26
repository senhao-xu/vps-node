package subscription

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchSubscription(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("ss://payload"))
		}))
		t.Cleanup(srv.Close)
		body, err := FetchSubscription(context.Background(), srv.URL)
		if err != nil || body != "ss://payload" {
			t.Fatalf("fetch: %q %v", body, err)
		}
	})

	t.Run("rejects non http schemes", func(t *testing.T) {
		for _, raw := range []string{"ftp://example.com/x", "file:///etc/passwd", "example.com/x", "http://"} {
			if _, err := FetchSubscription(context.Background(), raw); err == nil {
				t.Fatalf("expected error for %q", raw)
			}
		}
	})

	t.Run("rejects non-2xx", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(srv.Close)
		if _, err := FetchSubscription(context.Background(), srv.URL); err == nil {
			t.Fatal("expected error for 502 upstream")
		}
	})

	t.Run("caps response size", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("a", MaxFetchBytes+1)))
		}))
		t.Cleanup(srv.Close)
		if _, err := FetchSubscription(context.Background(), srv.URL); err == nil {
			t.Fatal("expected error for oversized response")
		}
	})

	t.Run("bounds redirects", func(t *testing.T) {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Endless redirect loop onto itself.
			http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
		}))
		t.Cleanup(srv.Close)
		if _, err := FetchSubscription(context.Background(), srv.URL); err == nil {
			t.Fatal("expected error for redirect loop")
		}
	})

	t.Run("honors context cancellation", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}))
		t.Cleanup(srv.Close)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := FetchSubscription(ctx, srv.URL); err == nil {
			t.Fatal("expected error for cancelled context")
		}
	})
}

func TestNormalizeFetchedContent(t *testing.T) {
	ssLink := "ss://aes-128-gcm:secret@1.2.3.4:8388#node-a"

	t.Run("clash yaml", func(t *testing.T) {
		content := "mixed-port: 7890\nproxies:\n  - {name: up-a, type: ss, server: a.example.com, port: 443, cipher: aes-128-gcm, password: x}\n  - {name: up-b, type: trojan, server: b.example.com, port: 443, password: y}\nproxy-groups: []\n"
		links, proxies := NormalizeFetchedContent(content)
		if len(links) != 0 || len(proxies) != 2 {
			t.Fatalf("clash yaml: links=%v proxies=%v", links, proxies)
		}
		if proxies[0]["name"] != "up-a" || proxies[1]["type"] != "trojan" {
			t.Fatalf("proxies: %v", proxies)
		}
	})

	t.Run("clash yaml with malformed proxy falls back to links", func(t *testing.T) {
		content := "proxies:\n  - {name: ''}\n"
		links, proxies := NormalizeFetchedContent(content)
		if proxies != nil {
			t.Fatalf("malformed proxies must not be accepted: %v", proxies)
		}
		if len(links) != 0 {
			t.Fatalf("no share links in a yaml document: %v", links)
		}
	})

	t.Run("base64 share links", func(t *testing.T) {
		content := base64.StdEncoding.EncodeToString([]byte(ssLink + "\nvless://uuid@b.example.com:443?security=none#node-b\n"))
		links, proxies := NormalizeFetchedContent(content)
		if len(proxies) != 0 || len(links) != 2 || links[0] != ssLink {
			t.Fatalf("base64: links=%v proxies=%v", links, proxies)
		}
	})

	t.Run("plain share links", func(t *testing.T) {
		content := "\n" + ssLink + "\n\nnot-a-link\n"
		links, _ := NormalizeFetchedContent(content)
		if len(links) != 1 || links[0] != ssLink {
			t.Fatalf("plain: %v", links)
		}
	})

	t.Run("empty", func(t *testing.T) {
		links, proxies := NormalizeFetchedContent("  \n")
		if links != nil || proxies != nil {
			t.Fatalf("empty: %v %v", links, proxies)
		}
	})
}

func TestSplitLinkLinesKeepsUnparseable(t *testing.T) {
	lines := SplitLinkLines("\nss://a@b:1#x\n\n garbage line \nvless://u@h:443#y\n")
	if len(lines) != 3 || lines[1] != "garbage line" {
		t.Fatalf("SplitLinkLines must keep every non-empty line verbatim: %v", lines)
	}
}

func ExampleNormalizeFetchedContent() {
	links, _ := NormalizeFetchedContent(base64.StdEncoding.EncodeToString([]byte("ss://m:p@a.example.com:443#n")))
	fmt.Println(links)
	// Output: [ss://m:p@a.example.com:443#n]
}
