package web_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestAgentHeartbeatObservedIP(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	server := e.seedServer(t, "server")
	key := e.agentKey(t, cookie, server)

	hb := func(xff string) {
		t.Helper()
		buf, err := json.Marshal(validHB())
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest("POST", e.ts.URL+"/api/agent/heartbeat", bytes.NewReader(buf))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+key)
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

	observed := func() string {
		t.Helper()
		resp, body := e.do(t, "GET", fmt.Sprintf("/api/servers/%d", server), nil, cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get server: %d %s", resp.StatusCode, body)
		}
		return jsonMap(t, body)["observed_ip"].(string)
	}

	hb("203.0.113.77")
	if got := observed(); got != "203.0.113.77" {
		t.Fatalf("observed_ip=%q want proxied public ip", got)
	}

	hb("172.20.0.9")
	if got := observed(); got != "" {
		t.Fatalf("observed_ip=%q want empty for private forwarded value", got)
	}

	hb("")
	if got := observed(); got != "" {
		t.Fatalf("observed_ip=%q want empty without headers", got)
	}
}
