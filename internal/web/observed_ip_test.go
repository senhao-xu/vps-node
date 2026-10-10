package web_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func newHeartbeatHarness(t *testing.T, e *testEnv) (hb func(xff string, extra map[string]any), observed func() string, server int64, cookie *http.Cookie) {
	t.Helper()
	cookie = e.login(t)
	server = e.seedServer(t, "server")
	agentKey := e.agentKey(t, cookie, server)

	hb = func(xff string, extra map[string]any) {
		t.Helper()
		body := validHB()
		for k, v := range extra {
			body[k] = v
		}
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest("POST", e.ts.URL+"/api/agent/heartbeat", bytes.NewReader(buf))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+agentKey)
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		resp, err := e.ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("heartbeat: %d", resp.StatusCode)
		}
	}

	observed = func() string {
		t.Helper()
		resp, body := e.do(t, "GET", fmt.Sprintf("/api/servers/%d", server), nil, cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get server: %d %s", resp.StatusCode, body)
		}
		return jsonMap(t, body)["observed_ip"].(string)
	}
	return hb, observed, server, cookie
}

func TestAgentHeartbeatObservedIP(t *testing.T) {
	e := newTestEnv(t)
	hb, observed, _, _ := newHeartbeatHarness(t, e)

	hb("203.0.113.77", nil)
	if got := observed(); got != "203.0.113.77" {
		t.Fatalf("observed_ip=%q want proxied public ip", got)
	}

	hb("172.20.0.9", nil)
	if got := observed(); got != "" {
		t.Fatalf("observed_ip=%q want empty for private forwarded value", got)
	}

	hb("", nil)
	if got := observed(); got != "" {
		t.Fatalf("observed_ip=%q want empty without headers", got)
	}
}

func TestAgentHeartbeatReportedPublicIP(t *testing.T) {
	e := newTestEnv(t)
	hb, observed, _, _ := newHeartbeatHarness(t, e)

	hb("203.0.113.77", map[string]any{"public_ip": "198.51.100.42"})
	if got := observed(); got != "198.51.100.42" {
		t.Fatalf("observed_ip=%q want agent-reported ip to win", got)
	}

	hb("203.0.113.77", map[string]any{"public_ip": "10.0.0.8"})
	if got := observed(); got != "203.0.113.77" {
		t.Fatalf("observed_ip=%q want fallback to xff on private report", got)
	}

	hb("", map[string]any{"public_ip": "not-an-ip"})
	if got := observed(); got != "" {
		t.Fatalf("observed_ip=%q want fallback to peer on invalid report", got)
	}

	hb("", map[string]any{"public_ip": "198.51.100.42"})
	if got := observed(); got != "198.51.100.42" {
		t.Fatalf("observed_ip=%q want agent-reported ip without headers", got)
	}
}

func TestAgentHeartbeatReportedIPv6(t *testing.T) {
	e := newTestEnv(t)
	hb, observed, server, cookie := newHeartbeatHarness(t, e)

	hb("", map[string]any{"public_ipv6": "2001:db8::42"})
	resp, body := e.do(t, "GET", fmt.Sprintf("/api/servers/%d", server), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get server: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if got := m["observed_ipv6"].(string); got != "2001:db8::42" {
		t.Fatalf("observed_ipv6=%q want agent-reported v6", got)
	}
	if got := m["ipv6"].(string); got != "" {
		t.Fatalf("manual ipv6=%q must stay untouched", got)
	}

	hb("", map[string]any{"public_ipv6": "fe80::1"})
	hb("", map[string]any{"public_ipv6": "fd00::1"})
	hb("", map[string]any{"public_ipv6": "not-a-v6"})
	hb("", map[string]any{"public_ipv6": "203.0.113.5"})
	if got := observed(); got != "" {
		t.Fatalf("observed_ip=%q must stay untouched by v6 reports", got)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d", server), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get server: %d %s", resp.StatusCode, body)
	}
	if got := jsonMap(t, body)["observed_ipv6"].(string); got != "" {
		t.Fatalf("observed_ipv6=%q want empty after invalid reports", got)
	}
}
