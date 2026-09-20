package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeChecker struct {
	failOn  string
	calls   []string
	version string
}

func (f *fakeChecker) Check(path string) error {
	f.calls = append(f.calls, path)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if f.failOn != "" && strings.Contains(string(data), f.failOn) {
		return errors.New("fake check failed")
	}
	return nil
}

func (f *fakeChecker) Version(ctx context.Context) (string, error) {
	return f.version, nil
}

func TestApplyHappyPath(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "sing-box", "config.json")
	checker := &fakeChecker{}
	applier := NewApplier(active, checker, nil, nil)

	err := applier.Apply(context.Background(), []byte(`{"inbounds":[]}`))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	data, err := os.ReadFile(active)
	if err != nil {
		t.Fatalf("read active config: %v", err)
	}
	if string(data) != `{"inbounds":[]}` {
		t.Fatalf("unexpected active config %q", data)
	}
	if len(checker.calls) != 1 {
		t.Fatalf("expected 1 check call, got %d", len(checker.calls))
	}
	if !strings.HasPrefix(filepath.Base(checker.calls[0]), ".singbox-apply-") {
		t.Fatalf("check must run on temp file, got %q", checker.calls[0])
	}
	info, err := os.Stat(active)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("active config perm = %o, want 600", perm)
	}
	entries, err := os.ReadDir(filepath.Dir(active))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %d entries", len(entries))
	}
}

func TestApplyRollbackKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "config.json")
	checker := &fakeChecker{}
	applier := NewApplier(active, checker, nil, nil)

	good := []byte(`{"valid":true}`)
	if err := applier.Apply(context.Background(), good); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	checker.failOn = `"invalid"`
	err := applier.Apply(context.Background(), []byte(`{"invalid":true}`))
	if err == nil {
		t.Fatal("expected check failure")
	}
	if !strings.Contains(err.Error(), "sing-box check rejected") {
		t.Fatalf("unexpected error %v", err)
	}
	data, err := os.ReadFile(active)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(good) {
		t.Fatalf("previous config must be retained, got %q", data)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("failed temp file must be removed, got %d entries", len(entries))
	}
}

func TestApplyReloadCommand(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "config.json")
	reload := &ReloadCommand{Bin: "touch", Args: []string{filepath.Join(dir, "reloaded")}}
	applier := NewApplier(active, &fakeChecker{}, reload, nil)

	if err := applier.Apply(context.Background(), []byte(`{}`)); err != nil {
		t.Fatalf("apply with reload: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "reloaded")); err != nil {
		t.Fatalf("reload command did not run: %v", err)
	}
}

func TestApplyReloadFailure(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "config.json")
	reload := &ReloadCommand{Bin: "false"}
	applier := NewApplier(active, &fakeChecker{}, reload, nil)

	err := applier.Apply(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected reload failure")
	}
	if !strings.Contains(err.Error(), "reload failed") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestParseReloadCommand(t *testing.T) {
	if cmd := mustParseReload(t, ""); cmd != nil {
		t.Fatalf("empty command must be nil, got %+v", cmd)
	}
	cmd := mustParseReload(t, "/bin/systemctl restart sing-box")
	if cmd.Bin != "/bin/systemctl" || len(cmd.Args) != 2 || cmd.Args[1] != "sing-box" {
		t.Fatalf("unexpected reload command %+v", cmd)
	}
	if _, err := ParseReloadCommand("sh -c 'rm -rf /'"); err == nil {
		t.Fatal("expected rejection of shell metacharacters")
	}
}

func mustParseReload(t *testing.T, raw string) *ReloadCommand {
	t.Helper()
	cmd, err := ParseReloadCommand(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return cmd
}

type errChecker struct{ err error }

func (e errChecker) Check(string) error                      { return e.err }
func (e errChecker) Version(context.Context) (string, error) { return "", fmt.Errorf("no version") }

func TestApplyCheckErrorContainsOutput(t *testing.T) {
	applier := NewApplier(filepath.Join(t.TempDir(), "config.json"),
		errChecker{errors.New("exit 1: unknown inbound")}, nil, nil)
	err := applier.Apply(context.Background(), []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "unknown inbound") {
		t.Fatalf("expected checker error surfaced, got %v", err)
	}
}
