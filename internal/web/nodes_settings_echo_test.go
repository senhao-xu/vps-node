package web_test

import (
	"crypto/ecdh"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNodeDetailEchoesPublicSettings(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")

	privateKey := testRealityPrivateKey(t)
	decoded, err := base64.RawURLEncoding.DecodeString(privateKey)
	if err != nil {
		t.Fatalf("decode private key: %v", err)
	}
	key, err := ecdh.X25519().NewPrivateKey(decoded)
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	wantPublicKey := base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "vless-1", "protocol": "vless", "port": 9443,
		"settings": map[string]any{
			"private_key": privateKey,
			"reality_settings": map[string]any{
				"server_name": "example.com", "server_port": 443, "short_id": "0123456789abcdef", "allow_insecure": true,
			},
		},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create vless node: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	if _, ok := created["settings"]; ok {
		t.Fatalf("create response must not include settings: %s", body)
	}
	vlessID := int64(created["id"].(float64))

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", vlessID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("node detail: %d %s", resp.StatusCode, body)
	}
	detail := jsonMap(t, body)
	settings, ok := detail["settings"].(map[string]any)
	if !ok {
		t.Fatalf("detail must include settings object: %s", body)
	}
	reality, ok := settings["reality_settings"].(map[string]any)
	if !ok {
		t.Fatalf("settings must include reality_settings: %s", body)
	}
	if reality["server_name"] != "example.com" || reality["short_id"] != "0123456789abcdef" ||
		reality["allow_insecure"] != true || reality["server_port"].(float64) != 443 {
		t.Fatalf("unexpected reality_settings echo: %v", reality)
	}
	if reality["public_key"] != wantPublicKey {
		t.Fatalf("public_key must be the key derived from the stored private key, got %v want %s", reality["public_key"], wantPublicKey)
	}
	assertNoSecrets(t, body, privateKey)

	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "ss-1", "protocol": "shadowsocks", "port": 8388,
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm", "password": "secret-password"},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create ss node: %d %s", resp.StatusCode, body)
	}
	ssID := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", ssID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ss node detail: %d %s", resp.StatusCode, body)
	}
	detail = jsonMap(t, body)
	settings, ok = detail["settings"].(map[string]any)
	if !ok || settings["cipher"] != "2022-blake3-aes-128-gcm" {
		t.Fatalf("ss settings must echo cipher: %s", body)
	}
	assertNoSecrets(t, body, "secret-password")

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes?server_id=%d", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("node list: %d %s", resp.StatusCode, body)
	}
	items := jsonMap(t, body)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(items))
	}
	for _, item := range items {
		if _, ok := item.(map[string]any)["settings"]; ok {
			t.Fatalf("list items must not include settings: %s", body)
		}
	}
}

func TestNodeDetailEchoesHysteria2PublicSettings(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")
	now := time.Now()
	cert, key := testTLSMaterial(t, "hy2.example.com", now.Add(-time.Hour), now.Add(time.Hour))

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "hy2-1", "protocol": "hysteria2", "port": 9443,
		"settings": map[string]any{
			"tls":         map[string]any{"server_name": "hy2.example.com", "allow_insecure": true},
			"certificate": cert, "private_key": key,
			"bandwidth":    map[string]any{"up": 100, "down": 200},
			"obfs":         map[string]any{"open": true, "type": "salamander", "password": "obfs-secret"},
			"hop_interval": "30000-40000",
		},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create hy2 node: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", nodeID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("node detail: %d %s", resp.StatusCode, body)
	}
	detail := jsonMap(t, body)
	settings, ok := detail["settings"].(map[string]any)
	if !ok {
		t.Fatalf("detail must include settings object: %s", body)
	}
	tlsSettings, ok := settings["tls"].(map[string]any)
	if !ok || tlsSettings["server_name"] != "hy2.example.com" || tlsSettings["allow_insecure"] != true {
		t.Fatalf("unexpected tls echo: %v", settings["tls"])
	}
	bandwidth, ok := settings["bandwidth"].(map[string]any)
	if !ok || bandwidth["up"].(float64) != 100 || bandwidth["down"].(float64) != 200 {
		t.Fatalf("unexpected bandwidth echo: %v", settings["bandwidth"])
	}
	obfs, ok := settings["obfs"].(map[string]any)
	if !ok || obfs["open"] != true || obfs["type"] != "salamander" || obfs["password"] != "obfs-secret" {
		t.Fatalf("unexpected obfs echo: %v", settings["obfs"])
	}
	if settings["hop_interval"] != "30000-40000" {
		t.Fatalf("unexpected hop_interval echo: %v", settings["hop_interval"])
	}
	for _, forbidden := range []string{"certificate", "private_key"} {
		if _, ok := settings[forbidden]; ok {
			t.Fatalf("settings must not include %q: %s", forbidden, body)
		}
	}
	if strings.Contains(body, "BEGIN CERTIFICATE") || strings.Contains(body, "BEGIN RSA PRIVATE KEY") {
		t.Fatalf("detail must not echo PEM material: %s", body)
	}
}
