package web_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"vps-node/internal/repo"
	"vps-node/internal/secrets"
	"vps-node/internal/singbox"
)

func TestManagedShadowsocksCipherValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "managed-ss")
	for i, cipher := range []string{singbox.SSMethod2022Chacha20, singbox.SSMethod2022Aes128Gcm, singbox.SSMethod2022Aes256Gcm} {
		before := e.revision(t, serverID)
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "address": "ss.example.com", "name": fmt.Sprintf("ss-%d", i), "protocol": "shadowsocks", "port": 8388 + i,
			"settings": map[string]any{"cipher": cipher, "password": "original-key"},
		}, cookie)
		if cipher == singbox.SSMethod2022Chacha20 {
			if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" || !strings.Contains(body, "managed shadowsocks cipher") {
				t.Fatalf("managed ChaCha create must be 422 validation, got %d %s", resp.StatusCode, body)
			}
			if e.revision(t, serverID) != before {
				t.Fatal("rejected create changed server revision")
			}
			continue
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("supported %s create: %d %s", cipher, resp.StatusCode, body)
		}
		nodeID := int64(jsonMap(t, body)["id"].(float64))
		stored, err := e.repo.GetNode(context.Background(), nodeID)
		if err != nil {
			t.Fatal(err)
		}
		before = e.revision(t, serverID)
		resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"settings": map[string]any{"cipher": singbox.SSMethod2022Chacha20}}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("managed ChaCha update must be 422 validation, got %d %s", resp.StatusCode, body)
		}
		after, err := e.repo.GetNode(context.Background(), nodeID)
		if err != nil || after.ProtocolSettings != stored.ProtocolSettings || !bytes.Equal(after.SecretEnc, stored.SecretEnc) || e.revision(t, serverID) != before {
			t.Fatalf("rejected update changed cipher, secret or revision: %v", err)
		}
	}
}

func TestPersistedUnsupportedManagedCipherRequiresExplicitReplacement(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "persisted-chacha")
	secret, err := secrets.Encrypt(e.appKey, []byte(`{"password":"original-key"}`))
	if err != nil {
		t.Fatal(err)
	}
	nodeID, err := e.repo.CreateNodeAndBump(context.Background(), repo.NewNode{
		ServerID: serverID, Address: "ss.example.com", Name: "ss", Protocol: repo.ProtocolShadowsocks, Port: 8388,
		ProtocolSettings: `{"cipher":"2022-blake3-chacha20-poly1305"}`, SecretEnc: secret, Status: repo.NodeStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := e.repo.GetNode(context.Background(), nodeID)
	if err != nil {
		t.Fatal(err)
	}
	revision := e.revision(t, serverID)
	resp, body := e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"name": "renamed"}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" || !strings.Contains(body, "managed shadowsocks cipher") {
		t.Fatalf("persisted unsupported cipher must be rejected on patch: %d %s", resp.StatusCode, body)
	}
	token := e.agentKey(t, cookie, serverID)
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, token)
	if resp.StatusCode != http.StatusInternalServerError || errorCode(t, body) != "internal" {
		t.Fatalf("persisted invalid cipher must reject agent config: %d %s", resp.StatusCode, body)
	}
	after, err := e.repo.GetNode(context.Background(), nodeID)
	if err != nil || after.ProtocolSettings != before.ProtocolSettings || !bytes.Equal(after.SecretEnc, before.SecretEnc) || e.revision(t, serverID) != revision {
		t.Fatalf("validation/rendering changed the persisted node: %v", err)
	}
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"settings": map[string]any{"cipher": singbox.SSMethod2022Aes256Gcm}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("explicit supported cipher replacement must work: %d %s", resp.StatusCode, body)
	}
}
