//go:build integration && with_quic && with_utls

package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"vps-node/internal/agentclient"
	kernelsingbox "vps-node/internal/kernel/singbox"
)

func userRefs(users []agentclient.User) []kernelsingbox.UserRef {
	out := make([]kernelsingbox.UserRef, 0, len(users))
	for _, u := range users {
		ref := kernelsingbox.UserRef{ID: u.ID, Nodes: make([]kernelsingbox.NodeRef, 0, len(u.Nodes))}
		for _, n := range u.Nodes {
			ref.Nodes = append(ref.Nodes, kernelsingbox.NodeRef{ID: n.ID, Protocol: n.Protocol, Port: n.Port})
		}
		out = append(out, ref)
	}
	return out
}

func TestEmbeddedSingBoxRuntimeStartStop(t *testing.T) {
	env, _, registerToken := seedProtocols(t)
	_, cfgResp := fetchRenderedConfig(t, env, registerToken)

	runtime := kernelsingbox.NewRuntime()
	if err := runtime.Start(cfgResp.Config.Singbox, userRefs(cfgResp.Users)); err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			t.Skipf("node listen ports already in use in this environment: %v", err)
		}
		t.Fatalf("start embedded sing-box: %v", err)
	}
	defer func() {
		if err := runtime.Stop(); err != nil {
			t.Fatalf("stop embedded sing-box: %v", err)
		}
	}()
	if snapshot := runtime.Snapshot(); snapshot.Traffic == nil {
		t.Fatal("snapshot traffic map must not be nil")
	}
}

// TestEmbeddedSingBoxRuntimeStartsRealityChain starts (not just validates) an
// entry config whose chain exit is a VLESS Reality node. sing-box refuses to
// build a reality client outbound without uTLS, so this reproduces the
// "uTLS is required by reality client" build failure that schema validation
// alone does not catch.
func TestEmbeddedSingBoxRuntimeStartsRealityChain(t *testing.T) {
	env, cookie, _ := seedProtocols(t)

	_, out := env.do("GET", "/api/nodes?page_size=100", nil, cookie)
	items, _ := out["items"].([]any)
	var vlessExitID int64
	for _, raw := range items {
		if n := raw.(map[string]any); n["protocol"] == "vless" {
			vlessExitID = int64(n["id"].(float64))
		}
	}
	if vlessExitID == 0 {
		t.Fatal("seedProtocols must seed a vless reality exit")
	}

	_, out = env.do("POST", "/api/servers", map[string]string{"name": "REALITY-ENTRY"}, cookie)
	entryServerID := int64(out["id"].(float64))
	_, out = env.do("POST", fmt.Sprintf("/api/servers/%d/agent-key", entryServerID), nil, cookie)
	entryKey, _ := out["agent_key"].(string)

	resp, created := env.do("POST", "/api/nodes", map[string]any{
		"server_id": entryServerID, "address": "reality-entry.example.com", "name": "reality-entry",
		"protocol": "shadowsocks", "port": 33000,
		"settings":      map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		"chain_node_id": vlessExitID,
	}, cookie)
	if resp.StatusCode != 201 {
		t.Fatalf("create reality chained entry: %v %v", resp.StatusCode, created)
	}

	client, err := agentclient.New(env.ts.URL)
	if err != nil {
		t.Fatalf("agent client: %v", err)
	}
	client.SetKey(entryKey)
	cfgResp, err := client.Config(context.Background(), 0)
	if err != nil {
		t.Fatalf("entry config poll: %v", err)
	}
	if cfgResp.Config == nil {
		t.Fatalf("expected entry config: %+v", cfgResp)
	}

	runtime := kernelsingbox.NewRuntime()
	if err := runtime.Start(cfgResp.Config.Singbox, userRefs(cfgResp.Users)); err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			t.Skipf("entry listen port already in use in this environment: %v", err)
		}
		t.Fatalf("start embedded sing-box for reality chain: %v", err)
	}
	defer func() { _ = runtime.Stop() }()
}
