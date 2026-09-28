//go:build integration

package e2e

import (
	"context"
	"fmt"
	"testing"

	"vps-node/internal/agentclient"
	kernelsingbox "vps-node/internal/kernel/singbox"
)

// TestEmbeddedSingBoxValidatesChainedPayload proves the panel-rendered chain
// configuration (chain client outbounds + relay pseudo users) loads in the
// embedded sing-box runtime for every exit protocol.
func TestEmbeddedSingBoxValidatesChainedPayload(t *testing.T) {
	env, cookie, exitKey := seedProtocols(t)

	// The seeded server (one node per protocol) plays the exit role; its
	// agent key comes from seedProtocols. Discover its nodes for chaining.
	_, out := env.do("GET", "/api/nodes?page_size=100", nil, cookie)
	items, _ := out["items"].([]any)
	var entryServerID int64
	exitIDs := map[string]int64{}
	for _, raw := range items {
		n := raw.(map[string]any)
		entryServerID = int64(n["server_id"].(float64))
		exitIDs[n["protocol"].(string)] = int64(n["id"].(float64))
	}

	// Second entry server chains one node at every protocol exit.
	_, out = env.do("POST", "/api/servers", map[string]string{"name": "ENTRY-02"}, cookie)
	relayServerID := int64(out["id"].(float64))
	_, out = env.do("POST", fmt.Sprintf("/api/servers/%d/agent-key", relayServerID), nil, cookie)
	relayKey, _ := out["agent_key"].(string)

	port := 31000
	for _, protocol := range []string{"shadowsocks", "vless", "hysteria2", "anytls", "socks", "http"} {
		port++
		resp, out := env.do("POST", "/api/nodes", map[string]any{
			"server_id": relayServerID, "address": "relay.example.com", "name": "relay-" + protocol,
			"protocol": "shadowsocks", "port": port,
			"settings":      map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
			"chain_node_id": exitIDs[protocol],
		}, cookie)
		if resp.StatusCode != 201 {
			t.Fatalf("create chained node for %s: %v %v", protocol, resp.StatusCode, out)
		}
	}

	// Entry-02 config: one chain outbound per exit protocol; must load in embedded sing-box.
	relayClient, err := agentclient.New(env.ts.URL)
	if err != nil {
		t.Fatalf("agent client: %v", err)
	}
	relayClient.SetKey(relayKey)
	cfgResp, err := relayClient.Config(context.Background(), 0)
	if err != nil {
		t.Fatalf("relay config poll: %v", err)
	}
	if cfgResp.Config == nil {
		t.Fatalf("expected config for relay server: %+v", cfgResp)
	}
	if err := kernelsingbox.Validate(cfgResp.Config.Singbox); err != nil {
		t.Fatalf("embedded sing-box rejected chained entry config: %v", err)
	}

	// Exit server config: relay pseudo users injected; must load too.
	exitClient, err := agentclient.New(env.ts.URL)
	if err != nil {
		t.Fatalf("agent client: %v", err)
	}
	exitClient.SetKey(exitKey)
	cfgResp, err = exitClient.Config(context.Background(), 0)
	if err != nil {
		t.Fatalf("exit config poll: %v", err)
	}
	if cfgResp.Config == nil {
		t.Fatalf("expected config for exit server: %+v", cfgResp)
	}
	if err := kernelsingbox.Validate(cfgResp.Config.Singbox); err != nil {
		t.Fatalf("embedded sing-box rejected chained exit config: %v", err)
	}

	t.Logf("entry server %d and relay server %d chained configs validated", entryServerID, relayServerID)
}

// TestEmbeddedSingBoxValidatesExternalExitPayload proves the panel-rendered
// external (custom-node line) chain outbounds load in the embedded sing-box
// runtime. The line credentials come from the source's stored content, so no
// relay user is injected anywhere.
func TestEmbeddedSingBoxValidatesExternalExitPayload(t *testing.T) {
	env, cookie, _ := seedProtocols(t)

	_, out := env.do("POST", "/api/servers", map[string]string{"name": "EXT-ENTRY"}, cookie)
	entryServerID := int64(out["id"].(float64))
	_, out = env.do("POST", fmt.Sprintf("/api/servers/%d/agent-key", entryServerID), nil, cookie)
	entryKey, _ := out["agent_key"].(string)

	_, out = env.do("POST", "/api/custom-nodes", map[string]any{
		"name": "e2e-external", "source_type": "links",
		"content": "ss://aes-128-gcm:ext-secret@203.0.113.9:8388#E2E-SS\n" +
			"trojan://tpass@203.0.113.10:443?sni=t.example.com#E2E-TROJAN\n" +
			"vless://11111111-1111-1111-1111-111111111111@203.0.113.11:443#E2E-VLESS\n" +
			"hysteria2://hypass@203.0.113.12:443?sni=h.example.com#E2E-HY2\n" +
			"anytls://atpass@203.0.113.13:443?sni=a.example.com#E2E-ANYTLS\n" +
			"http://u-9:uuid-9@203.0.113.14:8080#E2E-HTTP",
	}, cookie)
	sourceID := int64(out["id"].(float64))

	_, out = env.do("GET", fmt.Sprintf("/api/custom-nodes/%d/nodes", sourceID), nil, cookie)
	entries := out["entries"].([]any)
	if len(entries) != 6 {
		t.Fatalf("expected 6 external entries, got %v", out)
	}
	port := 32000
	for _, raw := range entries {
		entry := raw.(map[string]any)
		port++
		resp, created := env.do("POST", "/api/nodes", map[string]any{
			"server_id": entryServerID, "address": "ext-entry.example.com", "name": "ext-" + entry["name"].(string),
			"protocol": "shadowsocks", "port": port,
			"settings":               map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
			"chain_custom_node_id":   sourceID,
			"chain_custom_entry_key": entry["key"].(string),
		}, cookie)
		if resp.StatusCode != 201 {
			t.Fatalf("create external entry for %v: %v %v", entry["name"], resp.StatusCode, created)
		}
	}

	client, err := agentclient.New(env.ts.URL)
	if err != nil {
		t.Fatalf("agent client: %v", err)
	}
	client.SetKey(entryKey)
	cfgResp, err := client.Config(context.Background(), 0)
	if err != nil {
		t.Fatalf("external entry config poll: %v", err)
	}
	if cfgResp.Config == nil {
		t.Fatalf("expected config for external entry server: %+v", cfgResp)
	}
	if err := kernelsingbox.Validate(cfgResp.Config.Singbox); err != nil {
		t.Fatalf("embedded sing-box rejected external exit config: %v", err)
	}
}
