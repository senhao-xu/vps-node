package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestNodeUpdateChangesProtocolAndReplacesSettings(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "protocol-change")
	cert, key := testTLSMaterial(t, "proxy.example.com", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	nodeID := createHTTPNode(t, e, cookie, serverID, "proxy", 8443, map[string]any{
		"tls":         map[string]any{"server_name": "proxy.example.com"},
		"certificate": cert, "private_key": key,
	}, nil)
	path := fmt.Sprintf("/api/nodes/%d", nodeID)
	before := e.revision(t, serverID)

	for _, tc := range []struct {
		name   string
		body   map[string]any
		status int
	}{
		{"unknown protocol", map[string]any{"protocol": "unknown"}, http.StatusBadRequest},
		{"missing new protocol settings", map[string]any{"protocol": "vless", "settings": map[string]any{}}, http.StatusUnprocessableEntity},
		{"old settings rejected for new protocol", map[string]any{"protocol": "socks", "settings": map[string]any{"tls": map[string]any{"server_name": "proxy.example.com"}}}, http.StatusUnprocessableEntity},
	} {
		resp, body := e.do(t, "PUT", path, tc.body, cookie)
		if resp.StatusCode != tc.status {
			t.Fatalf("%s: got %d %s, want %d", tc.name, resp.StatusCode, body, tc.status)
		}
		if e.revision(t, serverID) != before {
			t.Fatalf("%s: rejected update bumped revision", tc.name)
		}
	}
	stored, err := e.repo.GetNode(context.Background(), nodeID)
	if err != nil || stored.Protocol != "http" || len(stored.SecretEnc) == 0 {
		t.Fatalf("rejected updates changed original node: %+v, %v", stored, err)
	}

	resp, body := e.do(t, "PUT", path, map[string]any{
		"protocol": "socks", "settings": map[string]any{}, "status": "active",
	}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["protocol"] != "socks" {
		t.Fatalf("switch to socks: %d %s", resp.StatusCode, body)
	}
	stored, err = e.repo.GetNode(context.Background(), nodeID)
	if err != nil || stored.Protocol != "socks" || stored.ProtocolSettings != "{}" || len(stored.SecretEnc) != 0 {
		t.Fatalf("old HTTP settings or secrets survived protocol change: %+v, %v", stored, err)
	}
	if e.revision(t, serverID) <= before {
		t.Fatal("protocol change must bump the owning server revision")
	}

	resp, body = e.do(t, "PUT", path, map[string]any{"protocol": "http", "settings": map[string]any{}}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["protocol"] != "http" {
		t.Fatalf("switch back to HTTP: %d %s", resp.StatusCode, body)
	}
	stored, err = e.repo.GetNode(context.Background(), nodeID)
	if err != nil || stored.ProtocolSettings != "{}" || len(stored.SecretEnc) != 0 {
		t.Fatalf("switch back must not restore old TLS material: %+v, %v", stored, err)
	}

	privateKey := testRealityPrivateKey(t)
	resp, body = e.do(t, "PUT", path, map[string]any{
		"protocol": "vless",
		"settings": map[string]any{
			"private_key":      privateKey,
			"reality_settings": map[string]any{"server_name": "reality.example.com"},
		},
	}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["protocol"] != "vless" {
		t.Fatalf("switch to VLESS: %d %s", resp.StatusCode, body)
	}
	stored, err = e.repo.GetNode(context.Background(), nodeID)
	if err != nil || stored.Protocol != "vless" || len(stored.SecretEnc) == 0 {
		t.Fatalf("new protocol secret was not saved: %+v, %v", stored, err)
	}
	resp, body = e.do(t, "GET", path, nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["settings"].(map[string]any)["reality_settings"].(map[string]any)["server_name"] != "reality.example.com" {
		t.Fatalf("VLESS settings missing after update: %d %s", resp.StatusCode, body)
	}
	assertNoSecrets(t, body, privateKey)
}
