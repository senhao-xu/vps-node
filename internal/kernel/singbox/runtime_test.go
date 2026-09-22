package singbox

import "testing"

const minimalConfig = `{"log":{"disabled":true},"outbounds":[{"type":"direct","tag":"direct"}]}`

func TestRuntimeStartReloadStop(t *testing.T) {
	runtime := NewRuntime()
	if err := runtime.Start([]byte(minimalConfig), nil); err != nil {
		t.Fatalf("start: %v", err)
	}
	if snapshot := runtime.Snapshot(); snapshot.Traffic == nil {
		t.Fatal("snapshot traffic map must not be nil")
	}
	if err := runtime.Start([]byte(minimalConfig), nil); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("second stop must be a no-op: %v", err)
	}
	if snapshot := runtime.Snapshot(); len(snapshot.Traffic) != 0 {
		t.Fatalf("stopped runtime must expose empty traffic, got %+v", snapshot.Traffic)
	}
}

func TestRuntimeRejectsInvalidConfig(t *testing.T) {
	runtime := NewRuntime()
	if err := runtime.Start([]byte(`{"outbounds":`), nil); err == nil {
		t.Fatal("malformed json must be rejected")
	}
	if err := runtime.Start([]byte(`{"inbounds":[{"type":"missing-protocol"}]}`), nil); err == nil {
		t.Fatal("unknown inbound type must be rejected")
	}
}

func TestValidateAcceptsMinimalConfig(t *testing.T) {
	if err := Validate([]byte(minimalConfig)); err != nil {
		t.Fatalf("validate minimal config: %v", err)
	}
	if err := Validate([]byte(`{"inbounds":`)); err == nil {
		t.Fatal("validate must reject malformed json")
	}
}

func TestPrepareConfigStripsExperimental(t *testing.T) {
	prepared, err := prepareConfig([]byte(`{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090"}},"outbounds":[]}`))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if got := string(prepared); got != `{"outbounds":[]}` {
		t.Fatalf("experimental must be stripped, got %s", got)
	}
	if _, err := prepareConfig([]byte(`{"outbounds":[]}`)); err != nil {
		t.Fatalf("config without experimental: %v", err)
	}
}
