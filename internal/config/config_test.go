package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"vps-node/internal/config"
)

const validAppKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func writePanelConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "panel.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadPanelFromDefaults(t *testing.T) {
	t.Setenv("PANEL_APP_KEY", validAppKey)

	cfg, err := config.LoadPanel("")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Listen != ":8080" {
		t.Fatalf("expected default listen :8080, got %s", cfg.Listen)
	}
	if cfg.DBPath != "data/panel.db" {
		t.Fatalf("expected default db path, got %s", cfg.DBPath)
	}
	if cfg.Retention.RawLogDays != 7 || cfg.Retention.AggregateDays != 90 {
		t.Fatalf("unexpected retention defaults: %+v", cfg.Retention)
	}
	if cfg.Retention.MaxConnectionLogs != 1_000_000 || cfg.Retention.MaxTrafficRecords != 5_000_000 {
		t.Fatalf("unexpected storage cap defaults: %+v", cfg.Retention)
	}
	if len(cfg.AppKey()) != 32 {
		t.Fatalf("expected 32-byte app key, got %d", len(cfg.AppKey()))
	}
}

func TestLoadPanelFromFile(t *testing.T) {
	path := writePanelConfig(t, `
listen: ":9090"
db_path: /tmp/panel.db
log_level: debug
app_key: `+validAppKey+`
retention:
  raw_log_days: 14
  aggregate_days: 180
  max_connection_logs: 500000
  max_traffic_records: 0
`)

	cfg, err := config.LoadPanel(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Listen != ":9090" || cfg.DBPath != "/tmp/panel.db" || cfg.LogLevel != "debug" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.Retention.RawLogDays != 14 || cfg.Retention.AggregateDays != 180 {
		t.Fatalf("unexpected retention: %+v", cfg.Retention)
	}
	if cfg.Retention.MaxConnectionLogs != 500_000 || cfg.Retention.MaxTrafficRecords != 0 {
		t.Fatalf("unexpected storage caps: %+v", cfg.Retention)
	}
}

func TestLoadPanelEnvOverridesFile(t *testing.T) {
	path := writePanelConfig(t, "listen: \":9090\"\napp_key: "+validAppKey+"\n")
	t.Setenv("PANEL_LISTEN", ":7070")

	cfg, err := config.LoadPanel(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Listen != ":7070" {
		t.Fatalf("expected env override :7070, got %s", cfg.Listen)
	}
}

func TestLoadPanelMissingAppKey(t *testing.T) {
	if _, err := config.LoadPanel(""); err == nil {
		t.Fatal("expected error for missing app_key")
	}
}

func TestLoadPanelBadAppKey(t *testing.T) {
	t.Setenv("PANEL_APP_KEY", "not-hex")
	if _, err := config.LoadPanel(""); err == nil {
		t.Fatal("expected error for bad app_key")
	}
}

func TestLoadPanelShortAppKey(t *testing.T) {
	t.Setenv("PANEL_APP_KEY", "aabbccdd")
	if _, err := config.LoadPanel(""); err == nil {
		t.Fatal("expected error for short app_key")
	}
}

func TestLoadPanelInvalidLogLevel(t *testing.T) {
	t.Setenv("PANEL_APP_KEY", validAppKey)
	t.Setenv("PANEL_LOG_LEVEL", "verbose")
	if _, err := config.LoadPanel(""); err == nil {
		t.Fatal("expected error for invalid log level")
	}
}

func writeAgentConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadAgentFromFile(t *testing.T) {
	path := writeAgentConfig(t, `
panel_url: https://panel.example.com
token: secret-token
server_id: 3
log_level: warn
heartbeat_interval: 15
sync_interval: 45
traffic_interval: 120
`)

	cfg, err := config.LoadAgent(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.PanelURL != "https://panel.example.com" || cfg.ServerID != 3 || cfg.LogLevel != "warn" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if cfg.HeartbeatInterval.Seconds() != 15 || cfg.SyncInterval.Seconds() != 45 || cfg.TrafficInterval.Seconds() != 120 {
		t.Fatalf("unexpected intervals: %+v", cfg)
	}
}

func TestLoadAgentDefaults(t *testing.T) {
	path := writeAgentConfig(t, "panel_url: https://p.example.com\ntoken: t\nserver_id: 1\n")

	cfg, err := config.LoadAgent(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.HeartbeatInterval.Seconds() != 30 || cfg.SyncInterval.Seconds() != 30 || cfg.TrafficInterval.Seconds() != 60 {
		t.Fatalf("unexpected interval defaults: %+v", cfg)
	}
}

func TestLoadAgentEnvOverrides(t *testing.T) {
	path := writeAgentConfig(t, "panel_url: https://p.example.com\ntoken: t\nserver_id: 1\n")
	t.Setenv("AGENT_SERVER_ID", "9")
	t.Setenv("AGENT_TOKEN", "env-token")

	cfg, err := config.LoadAgent(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ServerID != 9 || cfg.Token != "env-token" {
		t.Fatalf("unexpected env override: %+v", cfg)
	}
}

func TestLoadAgentMissingFields(t *testing.T) {
	if _, err := config.LoadAgent(""); err == nil {
		t.Fatal("expected error for missing agent config file")
	}

	path := writeAgentConfig(t, "panel_url: https://p.example.com\ntoken: t\nserver_id: 0\n")
	if _, err := config.LoadAgent(path); err == nil {
		t.Fatal("expected error for zero server_id")
	}
}
