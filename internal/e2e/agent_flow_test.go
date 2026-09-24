package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"vps-node/internal/agentclient"
	"vps-node/internal/agentruntime"
	"vps-node/internal/agentstate"
	"vps-node/internal/config"
	kernelsingbox "vps-node/internal/kernel/singbox"
)

type stubKernel struct {
	mu       sync.Mutex
	started  int
	snapshot kernelsingbox.Snapshot
	visits   []kernelsingbox.Visit
}

func (k *stubKernel) Start([]byte, []kernelsingbox.UserRef) error {
	k.mu.Lock()
	k.started++
	k.mu.Unlock()
	return nil
}

func (k *stubKernel) Stop() error { return nil }

func (k *stubKernel) Snapshot() kernelsingbox.Snapshot {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.snapshot.Traffic == nil {
		return kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{}}
	}
	return k.snapshot
}

func (k *stubKernel) setSnapshot(snap kernelsingbox.Snapshot) {
	k.mu.Lock()
	k.snapshot = snap
	k.mu.Unlock()
}

func (k *stubKernel) DrainVisits() []kernelsingbox.Visit {
	k.mu.Lock()
	defer k.mu.Unlock()
	visits := k.visits
	k.visits = nil
	return visits
}

func (k *stubKernel) setVisits(visits []kernelsingbox.Visit) {
	k.mu.Lock()
	k.visits = visits
	k.mu.Unlock()
}

func TestAgentEndToEndFlow(t *testing.T) {
	env := newPanelEnv(t)
	cookie := env.login()
	_, nodeID, userID := env.seedScenario(cookie)

	_, out := env.do("POST", "/api/servers/1/agent-key", nil, cookie)
	agentKey, _ := out["agent_key"].(string)
	if agentKey == "" {
		t.Fatal("expected agent_key")
	}

	state := &agentstate.State{}
	client, err := agentclient.New(env.ts.URL)
	if err != nil {
		t.Fatalf("new agent client: %v", err)
	}
	client.SetKey(agentKey)

	kernel := &stubKernel{}
	loop := agentruntime.NewLoop(agentruntime.LoopOptions{
		Config: &config.Agent{
			PanelURL:   env.ts.URL,
			AgentKey:   agentKey,
			ServerID:   1,
			Collection: config.Collection{Traffic: true, Visits: true},
		},
		Client: client, State: state,
		Kernel: kernel, Metrics: nil, Version: "0.1.0-test",
	})
	if err := loop.EnsureIdentity(context.Background()); err != nil {
		t.Fatalf("ensure identity: %v", err)
	}
	if state.ServerID != 1 {
		t.Fatalf("unexpected identity: %+v", state)
	}
	hb, err := client.Heartbeat(context.Background(), agentclient.HeartbeatRequest{
		Version: "0.1.0-test", CPUPercent: 12.5, MemoryPercent: 40, DiskPercent: 55, UptimeSeconds: 98765,
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !hb.OK || hb.ServerRevision <= 0 {
		t.Fatalf("unexpected heartbeat response %+v", hb)
	}

	cfgResp, err := client.Config(context.Background(), 0)
	if err != nil {
		t.Fatalf("config poll: %v", err)
	}
	if cfgResp.Status != "updated" || cfgResp.Config == nil || len(cfgResp.Users) != 1 {
		t.Fatalf("unexpected config response status=%s users=%d", cfgResp.Status, len(cfgResp.Users))
	}
	revision := cfgResp.Revision

	loop.SyncOnce(context.Background())
	if kernel.started != 1 {
		t.Fatalf("expected the kernel to start once, got %d", kernel.started)
	}
	if state.AppliedRevision != revision {
		t.Fatalf("applied revision = %d, want %d", state.AppliedRevision, revision)
	}

	current, err := client.Config(context.Background(), revision)
	if err != nil {
		t.Fatalf("config poll after apply: %v", err)
	}
	if current.Status != "current" || current.Revision != revision {
		t.Fatalf("expected status current at revision %d, got %+v", revision, current.Status)
	}

	pair := kernelsingbox.Pair{UserID: userID, NodeID: nodeID}
	kernel.setSnapshot(kernelsingbox.Snapshot{
		Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{
			pair: {Upload: 100, Download: 200},
		},
		Devices: []kernelsingbox.Device{{
			UserID: userID, NodeID: nodeID, IPs: []string{"203.0.113.9"}, Online: 1,
		}},
	})
	loop.TelemetryOnce(context.Background())

	_, userOut := env.do("GET", fmt.Sprintf("/api/users/%d", userID), nil, cookie)
	if used := userOut["used_bytes"].(float64); used != 300 {
		t.Fatalf("user used_bytes = %v, want 300", used)
	}
	if online := userOut["online_count"].(float64); online != 1 {
		t.Fatalf("user online_count = %v, want 1", online)
	}

	_, devicesOut := env.do("GET", fmt.Sprintf("/api/users/%d/devices", userID), nil, cookie)
	items, _ := devicesOut["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 device, got %v", devicesOut)
	}
	if items[0].(map[string]any)["ip"] != "203.0.113.9" {
		t.Fatalf("unexpected device item %v", items[0])
	}

	kernel.setVisits([]kernelsingbox.Visit{{
		UserID: userID, NodeID: nodeID, DestHost: "Example.COM", DestPort: 443,
		Network: "tcp", ClientIP: "203.0.113.9", At: time.Now().UTC(),
	}})
	loop.TelemetryOnce(context.Background())

	_, visitsOut := env.do("GET", fmt.Sprintf("/api/visits?user_id=%d", userID), nil, cookie)
	visitItems, _ := visitsOut["items"].([]any)
	if len(visitItems) != 1 {
		t.Fatalf("expected 1 visit, got %v", visitsOut)
	}
	visit := visitItems[0].(map[string]any)
	if visit["dest_host"] != "example.com" || visit["dest_port"].(float64) != 443 || visit["network"] != "tcp" {
		t.Fatalf("unexpected visit item %v", visit)
	}
	if visit["username"] == "" || visit["node_name"] == "" || visit["server_name"] == "" {
		t.Fatalf("visit must include display names, got %v", visit)
	}

	loop.TelemetryOnce(context.Background())
	_, userOut = env.do("GET", fmt.Sprintf("/api/users/%d", userID), nil, cookie)
	if used := userOut["used_bytes"].(float64); used != 300 {
		t.Fatalf("unchanged counters must not double count, used_bytes = %v", used)
	}

	kernel.setSnapshot(kernelsingbox.Snapshot{
		Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{
			pair: {Upload: 150, Download: 260},
		},
	})
	loop.TelemetryOnce(context.Background())
	_, userOut = env.do("GET", fmt.Sprintf("/api/users/%d", userID), nil, cookie)
	if used := userOut["used_bytes"].(float64); used != 410 {
		t.Fatalf("user used_bytes = %v, want 410 (100+200+50+60)", used)
	}

	env.do("PUT", fmt.Sprintf("/api/users/%d", userID), map[string]any{"status": "disabled"}, cookie)
	afterDisable, err := client.Config(context.Background(), state.AppliedRevision)
	if err != nil {
		t.Fatalf("config poll after disable: %v", err)
	}
	if afterDisable.Status != "updated" {
		t.Fatalf("expected updated config after user disable, got %s", afterDisable.Status)
	}
	if len(afterDisable.Users) != 0 {
		t.Fatalf("disabled user must not be synced, got %d users", len(afterDisable.Users))
	}

	if _, err := client.Heartbeat(context.Background(), agentclient.HeartbeatRequest{
		Version: "0.1.0-test", CPUPercent: 12.5, MemoryPercent: 40, DiskPercent: 55, UptimeSeconds: 98765,
	}); err != nil {
		t.Fatalf("heartbeat with old token: %v", err)
	}
}
