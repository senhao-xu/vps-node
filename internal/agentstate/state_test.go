package agentstate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"vps-node/internal/agentstate"
)

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	loaded, err := agentstate.Load(path)
	if err != nil {
		t.Fatalf("load missing state: %v", err)
	}
	if loaded.AgentID != 0 || loaded.AppliedRevision != 0 {
		t.Fatalf("expected empty state, got %+v", loaded)
	}

	want := &agentstate.State{
		AgentID:         7,
		AgentToken:      "tok",
		ServerID:        3,
		AppliedRevision: 1024,
		TrafficBatchSeq: 42,
		LogBatchSeq:     17,
	}
	if err := agentstate.Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := agentstate.Load(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.AgentID != want.AgentID || got.AgentToken != want.AgentToken ||
		got.ServerID != want.ServerID || got.AppliedRevision != want.AppliedRevision ||
		got.TrafficBatchSeq != want.TrafficBatchSeq || got.LogBatchSeq != want.LogBatchSeq {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, want)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("updated_at must be set on save")
	}
}

func TestStateFilePerms(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := agentstate.Save(path, &agentstate.State{AgentID: 1}); err != nil {
		t.Fatalf("save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("state file perm = %o, want 600", perm)
	}
}

func TestStateSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "state.json")
	if err := agentstate.Save(path, &agentstate.State{AgentID: 1, AgentToken: "a"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := agentstate.Save(path, &agentstate.State{AgentID: 1, AgentToken: "b"}); err != nil {
		t.Fatalf("resave: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "state.json" {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	got, err := agentstate.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.AgentToken != "b" {
		t.Fatalf("expected latest state, got %+v", got)
	}
}

func TestLoadCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := agentstate.Load(path); err == nil {
		t.Fatal("expected error for corrupt state file")
	}
}
