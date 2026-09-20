package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vps-node/internal/agentclient"
	"vps-node/internal/agentruntime"
	"vps-node/internal/agentstate"
	"vps-node/internal/config"
)

type fakeClash struct {
	snapshot atomic.Value
	secret   string
}

func (f *fakeClash) setSnapshot(conns []map[string]any) {
	f.snapshot.Store(conns)
}

func (f *fakeClash) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /connections", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+f.secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conns, _ := f.snapshot.Load().([]map[string]any)
		if conns == nil {
			conns = []map[string]any{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"downloadTotal": 0,
			"uploadTotal":   0,
			"connections":   conns,
		})
	})
	return mux
}

func TestAgentEndToEndFlow(t *testing.T) {
	env := newPanelEnv(t)
	cookie := env.login()
	_, nodeID, userID := env.seedScenario(cookie)

	_, out := env.do("POST", fmt.Sprintf("/api/servers/1/register-token"), nil, cookie)
	registerToken, _ := out["register_token"].(string)
	if registerToken == "" {
		t.Fatal("expected register_token")
	}

	statePath := filepath.Join(t.TempDir(), "agent-state.json")
	state, err := agentstate.Load(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	client, err := agentclient.New(env.ts.URL)
	if err != nil {
		t.Fatalf("new agent client: %v", err)
	}

	fakeBin := installFakeSingBox(t)
	workDir := t.TempDir()
	activeConfig := filepath.Join(workDir, "sing-box", "config.json")
	applier := agentruntime.NewApplier(activeConfig, agentruntime.NewSingBoxChecker(fakeBin), nil, nil)

	loop := agentruntime.NewLoop(agentruntime.LoopOptions{
		Config: &config.Agent{
			PanelURL:      env.ts.URL,
			Token:         "",
			RegisterToken: registerToken,
			ServerID:      1,
			StatePath:     statePath,
			SingBox: config.SingBox{
				ConfigPath: activeConfig,
				CheckBin:   fakeBin,
			},
			Collection: config.Collection{Traffic: true, Sessions: true, ConnectionLogs: true},
		},
		Client: client, State: state, StatePath: statePath,
		Applier: applier, Metrics: nil, Version: "0.1.0-test",
	})
	if err := loop.EnsureIdentity(context.Background()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if state.AgentToken == "" || state.ServerID != 1 {
		t.Fatalf("unexpected identity after register: %+v", state)
	}
	if st, err := os.Stat(statePath); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("state file must exist with 0600, got %v %v", st, err)
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
	rendered := cfgResp.Config.Singbox

	if err := applier.Apply(context.Background(), rendered); err != nil {
		t.Fatalf("apply rendered config: %v", err)
	}
	stored, err := os.ReadFile(activeConfig)
	if err != nil {
		t.Fatalf("read applied config: %v", err)
	}
	var appliedCfg map[string]any
	if err := json.Unmarshal(stored, &appliedCfg); err != nil {
		t.Fatalf("applied config is not JSON: %v", err)
	}

	state.AppliedRevision = revision
	if err := agentstate.Save(statePath, state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	current, err := client.Config(context.Background(), revision)
	if err != nil {
		t.Fatalf("config poll after apply: %v", err)
	}
	if current.Status != "current" || current.Revision != revision {
		t.Fatalf("expected status current at revision %d, got %+v", revision, current.Status)
	}

	baseURL, secret, err := agentruntime.EndpointFromConfig(rendered)
	if err != nil {
		t.Fatalf("endpoint from config: %v", err)
	}
	if !strings.HasPrefix(baseURL, "http://127.0.0.1:") {
		t.Fatalf("unexpected clash endpoint %q", baseURL)
	}
	clash := &fakeClash{secret: secret}
	clashTS := httptest.NewServer(clash.handler())
	t.Cleanup(clashTS.Close)
	clashClient, err := agentruntime.NewClashClient(clashTS.URL, secret)
	if err != nil {
		t.Fatalf("new clash client: %v", err)
	}
	collector := agentruntime.NewCollector(clashClient, agentruntime.BuildTable(cfgResp.Users))

	start := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	clash.setSnapshot([]map[string]any{{
		"id": "conn-1", "upload": 100, "download": 200, "start": start,
		"metadata": map[string]any{
			"network": "tcp", "type": "Vless", "sourceIP": "203.0.113.9",
			"inbound":     fmt.Sprintf("vless-%d", nodeID),
			"inboundPort": "443",
			"inboundUser": fmt.Sprintf("u-%d", userID),
		},
	}})

	poll1, err := collector.Poll(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("poll 1: %v", err)
	}
	if len(poll1.Traffic) != 1 || poll1.Traffic[0].Upload != 100 || poll1.Traffic[0].Download != 200 {
		t.Fatalf("unexpected poll1 traffic %+v", poll1.Traffic)
	}
	if len(poll1.Sessions) != 1 || poll1.Sessions[0].IP != "203.0.113.9" {
		t.Fatalf("unexpected poll1 sessions %+v", poll1.Sessions)
	}

	clash.setSnapshot([]map[string]any{{
		"id": "conn-1", "upload": 150, "download": 260, "start": start,
		"metadata": map[string]any{
			"network": "tcp", "type": "Vless", "sourceIP": "203.0.113.9",
			"inbound":     fmt.Sprintf("vless-%d", nodeID),
			"inboundUser": fmt.Sprintf("u-%d", userID),
		},
	}})
	poll2, err := collector.Poll(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("poll 2: %v", err)
	}
	if len(poll2.Traffic) != 1 || poll2.Traffic[0].Upload != 50 || poll2.Traffic[0].Download != 60 {
		t.Fatalf("expected deltas 50/60, got %+v", poll2.Traffic)
	}

	now := time.Now().UTC()
	trafficBatch := agentclient.TrafficBatch{BatchSeq: state.TrafficBatchSeq + 1}
	for _, d := range poll1.Traffic {
		trafficBatch.Records = append(trafficBatch.Records, agentclient.TrafficRecord{
			UserID: d.UserID, NodeID: d.NodeID, UploadBytes: d.Upload, DownloadBytes: d.Download,
			RecordedAt: now.Format(time.RFC3339),
		})
	}
	ack1, err := client.Traffic(context.Background(), trafficBatch)
	if err != nil {
		t.Fatalf("traffic report: %v", err)
	}
	if !ack1.Accepted || ack1.Records != 1 {
		t.Fatalf("unexpected traffic ack %+v", ack1)
	}
	state.TrafficBatchSeq = trafficBatch.BatchSeq

	ackDup, err := client.Traffic(context.Background(), trafficBatch)
	if err != nil {
		t.Fatalf("duplicate traffic report: %v", err)
	}
	if !ackDup.Accepted {
		t.Fatalf("duplicate batch must be accepted idempotently, got %+v", ackDup)
	}

	_, userOut := env.do("GET", fmt.Sprintf("/api/users/%d", userID), nil, cookie)
	if used := userOut["used_bytes"].(float64); used != 300 {
		t.Fatalf("user used_bytes = %v, want 300 (exactly once accounting)", used)
	}

	sessionBatch := agentclient.SessionBatch{ReportedAt: now.Format(time.RFC3339)}
	for _, s := range poll2.Sessions {
		sessionBatch.Sessions = append(sessionBatch.Sessions, agentclient.SessionReport{
			UserID: s.UserID, NodeID: s.NodeID, IP: s.IP,
			UploadBytes: s.Upload, DownloadBytes: s.Download,
			ConnectedAt: s.ConnectedAt.Format(time.RFC3339),
			LastSeenAt:  s.LastSeenAt.Format(time.RFC3339),
		})
	}
	if _, err := client.Sessions(context.Background(), sessionBatch); err != nil {
		t.Fatalf("session report: %v", err)
	}
	_, sessionsOut := env.do("GET", fmt.Sprintf("/api/users/%d/sessions", userID), nil, cookie)
	items, _ := sessionsOut["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 session, got %v", sessionsOut)
	}
	if items[0].(map[string]any)["ip"] != "203.0.113.9" {
		t.Fatalf("unexpected session item %v", items[0])
	}

	clash.setSnapshot(nil)
	poll3, err := collector.Poll(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("poll 3: %v", err)
	}
	if len(poll3.Closed) != 1 || poll3.Closed[0].Upload != 150 || poll3.Closed[0].Download != 260 {
		t.Fatalf("closed log must carry connection lifetime totals, got %+v", poll3.Closed)
	}

	logBatch := agentclient.LogBatch{BatchSeq: state.LogBatchSeq + 1}
	for _, c := range poll3.Closed {
		closedAt := c.ClosedAt.Format(time.RFC3339)
		logBatch.Logs = append(logBatch.Logs, agentclient.ConnectionLog{
			UserID: c.UserID, NodeID: c.NodeID, IP: c.IP, Protocol: c.Protocol,
			UploadBytes: c.Upload, DownloadBytes: c.Download,
			ConnectedAt: c.ConnectedAt.Format(time.RFC3339),
			ClosedAt:    &closedAt, Status: "closed",
		})
	}
	logAck, err := client.ConnectionLogs(context.Background(), logBatch)
	if err != nil {
		t.Fatalf("log report: %v", err)
	}
	if !logAck.Accepted || logAck.Logs != 1 {
		t.Fatalf("unexpected log ack %+v", logAck)
	}
	if _, err := client.ConnectionLogs(context.Background(), logBatch); err != nil {
		t.Fatalf("duplicate log report: %v", err)
	}

	_, logsOut := env.do("GET", fmt.Sprintf("/api/users/%d/connection-logs?page_size=50", userID), nil, cookie)
	logItems, _ := logsOut["items"].([]any)
	if len(logItems) != 1 {
		t.Fatalf("expected exactly 1 connection log after duplicate batch, got %v", logsOut)
	}
	entry := logItems[0].(map[string]any)
	if entry["status"] != "closed" || entry["protocol"] != "vless" || entry["ip"] != "203.0.113.9" {
		t.Fatalf("unexpected log entry %v", entry)
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
