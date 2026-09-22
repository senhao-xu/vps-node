package agentruntime

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"vps-node/internal/agentclient"
	"vps-node/internal/agentstate"
)

func TestSyncPassForcesApplyOnStartup(t *testing.T) {
	const revision = 5
	var gotVersions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/config" {
			http.NotFound(w, r)
			return
		}
		version := r.URL.Query().Get("version")
		gotVersions = append(gotVersions, version)
		w.Header().Set("Content-Type", "application/json")
		if version == strconv.Itoa(revision) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "current", "revision": revision})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "updated",
			"revision": revision,
			"config":   map[string]any{"singbox": json.RawMessage(`{"inbounds":[]}`)},
		})
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	checker := &fakeChecker{}
	configPath := filepath.Join(t.TempDir(), "config.json")
	applier := NewApplier(configPath, checker, nil, nil)
	state := &agentstate.State{AppliedRevision: revision}
	loop := NewLoop(LoopOptions{
		Client:    client,
		State:     state,
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Applier:   applier,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	loop.syncPass(context.Background())

	if len(gotVersions) != 1 || gotVersions[0] != "" {
		t.Fatalf("startup sync must request the full config (no version), got %v", gotVersions)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("startup sync did not write config: %v", err)
	}
	if string(data) != `{"inbounds":[]}` {
		t.Fatalf("unexpected config: %s", data)
	}
	if state.AppliedRevision != revision {
		t.Fatalf("applied revision = %d, want %d", state.AppliedRevision, revision)
	}
	if len(checker.calls) != 1 {
		t.Fatalf("expected 1 apply, got %d", len(checker.calls))
	}

	loop.syncPass(context.Background())

	if len(gotVersions) != 2 || gotVersions[1] != strconv.Itoa(revision) {
		t.Fatalf("steady-state sync must send the applied revision, got %v", gotVersions)
	}
	if len(checker.calls) != 1 {
		t.Fatalf("current revision must not re-apply, got %d applies", len(checker.calls))
	}
}

func TestSyncPassRetriesForceUntilApplySucceeds(t *testing.T) {
	const revision = 3
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("version") == strconv.Itoa(revision) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "current", "revision": revision})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "updated",
			"revision": revision,
			"config":   map[string]any{"singbox": json.RawMessage(`{"inbounds":[]}`)},
		})
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	checker := &fakeChecker{failOn: "inbounds"}
	applier := NewApplier(filepath.Join(t.TempDir(), "config.json"), checker, nil, nil)
	state := &agentstate.State{AppliedRevision: revision}
	loop := NewLoop(LoopOptions{
		Client:    client,
		State:     state,
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Applier:   applier,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	loop.syncPass(context.Background())
	if !loop.forceApply {
		t.Fatal("failed startup apply must keep forcing on subsequent syncs")
	}

	checker.failOn = ""
	loop.syncPass(context.Background())
	if loop.forceApply {
		t.Fatal("successful apply must clear the force flag")
	}
	if state.AppliedRevision != revision {
		t.Fatalf("applied revision = %d, want %d", state.AppliedRevision, revision)
	}
	if len(checker.calls) != 2 {
		t.Fatalf("expected 2 apply attempts, got %d", len(checker.calls))
	}
}
