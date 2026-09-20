package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fastDelays(c *Client) {
	c.baseDelay = time.Nanosecond
	c.maxDelay = 2 * time.Nanosecond
}

func TestRegisterAndPayloadShape(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"agent_id": 7, "agent_token": "agent-tok", "server_id": 3,
			"heartbeat_interval_seconds": 30, "sync_interval_seconds": 30, "traffic_interval_seconds": 60,
		})
	}))
	defer ts.Close()

	c, err := New(ts.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	fastDelays(c)

	resp, err := c.Register(context.Background(), RegisterRequest{RegisterToken: "reg-tok", Version: "0.1.0"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if resp.AgentID != 7 || resp.AgentToken != "agent-tok" || resp.ServerID != 3 || resp.HeartbeatIntervalSeconds != 30 {
		t.Fatalf("unexpected register response %+v", resp)
	}
	if gotAuth != "" {
		t.Fatalf("register must not send a bearer token, got %q", gotAuth)
	}
	if gotBody["register_token"] != "reg-tok" || gotBody["version"] != "0.1.0" {
		t.Fatalf("unexpected register body %v", gotBody)
	}
}

func TestNoRetryOnClientErrors(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "unauthorized", "message": "bad token"}})
	}))
	defer ts.Close()

	c, _ := New(ts.URL)
	fastDelays(c)

	_, err := c.Heartbeat(context.Background(), HeartbeatRequest{Version: "0.1.0"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.Status != http.StatusUnauthorized || apiErr.Code != "unauthorized" {
		t.Fatalf("unexpected api error %+v", apiErr)
	}
	if calls.Load() != 1 {
		t.Fatalf("4xx must not be retried, got %d calls", calls.Load())
	}
}

func TestRetryOnServerErrorAndRateLimit(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch n {
		case 1:
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "internal", "message": "boom"}})
		case 2:
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "server_revision": 12, "heartbeat_interval_seconds": 30})
		}
	}))
	defer ts.Close()

	c, _ := New(ts.URL)
	fastDelays(c)

	resp, err := c.Heartbeat(context.Background(), HeartbeatRequest{Version: "0.1.0", CPUPercent: 1, MemoryPercent: 2, DiskPercent: 3, UptimeSeconds: 4})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !resp.OK || resp.ServerRevision != 12 {
		t.Fatalf("unexpected heartbeat response %+v", resp)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls.Load())
	}
}

func TestRetriesExhausted(t *testing.T) {
	var calls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	c, _ := New(ts.URL)
	fastDelays(c)

	_, err := c.Heartbeat(context.Background(), HeartbeatRequest{Version: "0.1.0"})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 4 {
		t.Fatalf("expected 4 attempts, got %d", calls.Load())
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadGateway {
		t.Fatalf("last error should be the final APIError, got %v", err)
	}
}

func TestNetworkErrorsRetry(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := ts.URL
	ts.Close()

	c, _ := New(url)
	fastDelays(c)

	_, err := c.Traffic(context.Background(), TrafficBatch{BatchSeq: 1})
	if err == nil || !strings.Contains(err.Error(), "giving up after 4 attempts") {
		t.Fatalf("expected exhausted retries over network errors, got %v", err)
	}
}

func TestBackoffDelayGrowth(t *testing.T) {
	c, _ := New("https://panel.example.com")
	c.randomize = func(int) int { return 0 }

	cases := []struct {
		failed int
		want   time.Duration
	}{
		{1, 500 * time.Millisecond},
		{2, time.Second},
		{3, 2 * time.Second},
		{4, 4 * time.Second},
		{10, 8 * time.Second},
	}
	for _, tc := range cases {
		if got := c.backoffDelay(tc.failed); got != tc.want {
			t.Fatalf("failed attempts %d: got %s, want %s", tc.failed, got, tc.want)
		}
	}
}

func TestNewRejectsInvalidBaseURL(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected error for empty base URL")
	}
	if _, err := New("not a url://x y"); err == nil {
		t.Fatal("expected error for invalid base URL")
	}
}
