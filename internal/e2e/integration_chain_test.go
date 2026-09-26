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
	for _, protocol := range []string{"shadowsocks", "vless", "hysteria2", "anytls"} {
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

	// Entry-02 config: four chain outbounds; must load in embedded sing-box.
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
