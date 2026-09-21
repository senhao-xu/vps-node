package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultListen        = ":8080"
	defaultDBPath        = "data/panel.db"
	defaultLogLevel      = "info"
	defaultRawLogDays    = 7
	defaultAggregateDays = 90
	defaultAdminUsername = "admin"
	appKeySize           = 32

	defaultHeartbeatInterval = 30 * time.Second
	defaultSyncInterval      = 30 * time.Second
	defaultTrafficInterval   = 60 * time.Second
	minAgentInterval         = 5 * time.Second

	defaultAgentStatePath    = "agent_state.json"
	defaultSingBoxConfigPath = "/etc/sing-box/config.json"
	defaultSingBoxCheckBin   = "sing-box"

	defaultSweepInterval = time.Hour
	minSweepInterval     = time.Second

	defaultMaxConnectionLogs = 1_000_000
	defaultMaxTrafficRecords = 5_000_000
)

// AppKeySettingKey is the settings-table key used to persist an
// auto-generated panel app_key.
const AppKeySettingKey = "app_key"

const (
	appKeySourceEnv      = "env"
	appKeySourceFile     = "file"
	appKeySourceDatabase = "database"
	appKeySourceAuto     = "auto"
)

type Retention struct {
	RawLogDays    int
	AggregateDays int
	SweepInterval time.Duration
	// MaxConnectionLogs and MaxTrafficRecords bound raw rows regardless of the
	// retention window; 0 disables the cap.
	MaxConnectionLogs int64
	MaxTrafficRecords int64
}

type Admin struct {
	Username string
	Password string
}

type Panel struct {
	Listen       string
	DBPath       string
	LogLevel     string
	Retention    Retention
	Admin        Admin
	SecureCookie bool
	appKey       []byte
	appKeySource string
}

func (p *Panel) AppKey() []byte {
	return append([]byte(nil), p.appKey...)
}

// AppKeySource reports where the app key came from:
// "env", "file", "database", "auto", or "" when unresolved.
func (p *Panel) AppKeySource() string {
	return p.appKeySource
}

// ResolveAppKey fills the panel app key when it was not provided via
// environment or config file. It first tries the persisted value (get
// returns "" when absent) and otherwise generates a fresh random key and
// persists it via set so restarts keep encrypting existing data.
func (p *Panel) ResolveAppKey(ctx context.Context, get func(context.Context) (string, error), set func(context.Context, string) error) error {
	if len(p.appKey) != 0 {
		return nil
	}
	stored, err := get(ctx)
	if err != nil {
		return fmt.Errorf("load persisted app_key: %w", err)
	}
	if stored != "" {
		key, err := decodeAppKey(stored)
		if err != nil {
			return fmt.Errorf("persisted app_key invalid: %w", err)
		}
		p.appKey = key
		p.appKeySource = appKeySourceDatabase
		return nil
	}
	key := make([]byte, appKeySize)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("generate app_key: %w", err)
	}
	if err := set(ctx, hex.EncodeToString(key)); err != nil {
		return fmt.Errorf("persist app_key: %w", err)
	}
	p.appKey = key
	p.appKeySource = appKeySourceAuto
	return nil
}

type SingBox struct {
	ConfigPath    string
	CheckBin      string
	ReloadCommand string
}

type Collection struct {
	Traffic        bool
	Sessions       bool
	ConnectionLogs bool
}

type Agent struct {
	PanelURL          string
	Token             string
	RegisterToken     string
	ServerID          int64
	StatePath         string
	LogLevel          string
	HeartbeatInterval time.Duration
	SyncInterval      time.Duration
	TrafficInterval   time.Duration
	SingBox           SingBox
	Collection        Collection
}

type panelFile struct {
	Listen       string `yaml:"listen"`
	DBPath       string `yaml:"db_path"`
	LogLevel     string `yaml:"log_level"`
	AppKey       string `yaml:"app_key"`
	SecureCookie bool   `yaml:"secure_cookie"`
	Admin        struct {
		Username string `yaml:"username"`
		Password string `yaml:"password"`
	} `yaml:"admin"`
	Retention struct {
		RawLogDays    int    `yaml:"raw_log_days"`
		AggregateDays int    `yaml:"aggregate_days"`
		SweepInterval int    `yaml:"sweep_interval_seconds"`
		MaxConnLogs   *int64 `yaml:"max_connection_logs"`
		MaxTraffic    *int64 `yaml:"max_traffic_records"`
	} `yaml:"retention"`
}

type agentFile struct {
	PanelURL             string `yaml:"panel_url"`
	Token                string `yaml:"token"`
	RegisterToken        string `yaml:"register_token"`
	ServerID             int64  `yaml:"server_id"`
	StatePath            string `yaml:"state_path"`
	LogLevel             string `yaml:"log_level"`
	HeartbeatIntervalSec int    `yaml:"heartbeat_interval"`
	SyncIntervalSec      int    `yaml:"sync_interval"`
	TrafficIntervalSec   int    `yaml:"traffic_interval"`
	SingBox              struct {
		ConfigPath    string `yaml:"config_path"`
		CheckBin      string `yaml:"check_bin"`
		ReloadCommand string `yaml:"reload_command"`
	} `yaml:"singbox"`
	Collection struct {
		Traffic        *bool `yaml:"traffic"`
		Sessions       *bool `yaml:"sessions"`
		ConnectionLogs *bool `yaml:"connection_logs"`
	} `yaml:"collection"`
}

func LoadPanel(path string) (*Panel, error) {
	cfg := &Panel{
		Listen:   defaultListen,
		DBPath:   defaultDBPath,
		LogLevel: defaultLogLevel,
		Retention: Retention{
			RawLogDays:        defaultRawLogDays,
			AggregateDays:     defaultAggregateDays,
			SweepInterval:     defaultSweepInterval,
			MaxConnectionLogs: defaultMaxConnectionLogs,
			MaxTrafficRecords: defaultMaxTrafficRecords,
		},
		Admin: Admin{Username: defaultAdminUsername},
	}

	fileAppKey := ""
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read panel config: %w", err)
		}
		var pf panelFile
		if err := yaml.Unmarshal(data, &pf); err != nil {
			return nil, fmt.Errorf("parse panel config: %w", err)
		}
		if pf.Listen != "" {
			cfg.Listen = pf.Listen
		}
		if pf.DBPath != "" {
			cfg.DBPath = pf.DBPath
		}
		if pf.LogLevel != "" {
			cfg.LogLevel = pf.LogLevel
		}
		if pf.Retention.RawLogDays > 0 {
			cfg.Retention.RawLogDays = pf.Retention.RawLogDays
		}
		if pf.Retention.AggregateDays > 0 {
			cfg.Retention.AggregateDays = pf.Retention.AggregateDays
		}
		if pf.Retention.SweepInterval > 0 {
			cfg.Retention.SweepInterval = time.Duration(pf.Retention.SweepInterval) * time.Second
		}
		if pf.Retention.MaxConnLogs != nil {
			cfg.Retention.MaxConnectionLogs = max64(*pf.Retention.MaxConnLogs, 0)
		}
		if pf.Retention.MaxTraffic != nil {
			cfg.Retention.MaxTrafficRecords = max64(*pf.Retention.MaxTraffic, 0)
		}
		if strings.TrimSpace(pf.Admin.Username) != "" {
			cfg.Admin.Username = strings.TrimSpace(pf.Admin.Username)
		}
		cfg.Admin.Password = pf.Admin.Password
		cfg.SecureCookie = pf.SecureCookie
		fileAppKey = strings.TrimSpace(pf.AppKey)
	}

	if v := os.Getenv("PANEL_LISTEN"); v != "" {
		cfg.Listen = v
	}
	if v := os.Getenv("PANEL_DB_PATH"); v != "" {
		cfg.DBPath = v
	}
	if v := os.Getenv("PANEL_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("PANEL_RETENTION_RAW_LOG_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("PANEL_RETENTION_RAW_LOG_DAYS: %w", err)
		}
		if n > 0 {
			cfg.Retention.RawLogDays = n
		}
	}
	if v := os.Getenv("PANEL_RETENTION_AGGREGATE_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("PANEL_RETENTION_AGGREGATE_DAYS: %w", err)
		}
		if n > 0 {
			cfg.Retention.AggregateDays = n
		}
	}
	if v := os.Getenv("PANEL_RETENTION_SWEEP_SECONDS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("PANEL_RETENTION_SWEEP_SECONDS: %w", err)
		}
		if n > 0 {
			cfg.Retention.SweepInterval = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("PANEL_RETENTION_MAX_CONNECTION_LOGS"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("PANEL_RETENTION_MAX_CONNECTION_LOGS: %w", err)
		}
		cfg.Retention.MaxConnectionLogs = max64(n, 0)
	}
	if v := os.Getenv("PANEL_RETENTION_MAX_TRAFFIC_RECORDS"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("PANEL_RETENTION_MAX_TRAFFIC_RECORDS: %w", err)
		}
		cfg.Retention.MaxTrafficRecords = max64(n, 0)
	}
	if v := os.Getenv("PANEL_ADMIN_USER"); v != "" {
		cfg.Admin.Username = strings.TrimSpace(v)
	}
	if v := os.Getenv("PANEL_ADMIN_PASSWORD"); v != "" {
		cfg.Admin.Password = v
	}
	if v := os.Getenv("PANEL_SECURE_COOKIES"); v != "" {
		enabled, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("PANEL_SECURE_COOKIES: %w", err)
		}
		cfg.SecureCookie = enabled
	}

	appKeyHex := firstNonEmpty(os.Getenv("PANEL_APP_KEY"), fileAppKey)
	if appKeyHex != "" {
		key, err := decodeAppKey(appKeyHex)
		if err != nil {
			return nil, err
		}
		cfg.appKey = key
		if os.Getenv("PANEL_APP_KEY") != "" {
			cfg.appKeySource = appKeySourceEnv
		} else {
			cfg.appKeySource = appKeySourceFile
		}
	}

	if cfg.Listen == "" {
		return nil, fmt.Errorf("listen address is required")
	}
	if cfg.DBPath == "" {
		return nil, fmt.Errorf("db_path is required")
	}
	if err := validateLogLevel(cfg.LogLevel); err != nil {
		return nil, err
	}
	if cfg.Retention.RawLogDays < 1 || cfg.Retention.AggregateDays < 1 {
		return nil, fmt.Errorf("retention days must be >= 1")
	}
	if cfg.Retention.SweepInterval < minSweepInterval {
		return nil, fmt.Errorf("retention sweep_interval_seconds must be >= %d", int(minSweepInterval/time.Second))
	}
	return cfg, nil
}

func LoadAgent(path string) (*Agent, error) {
	cfg := &Agent{
		LogLevel:          defaultLogLevel,
		HeartbeatInterval: defaultHeartbeatInterval,
		SyncInterval:      defaultSyncInterval,
		TrafficInterval:   defaultTrafficInterval,
		StatePath:         defaultAgentStatePath,
		SingBox: SingBox{
			ConfigPath: defaultSingBoxConfigPath,
			CheckBin:   defaultSingBoxCheckBin,
		},
		Collection: Collection{Traffic: true, Sessions: true, ConnectionLogs: true},
	}

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read agent config: %w", err)
		}
		var af agentFile
		if err := yaml.Unmarshal(data, &af); err != nil {
			return nil, fmt.Errorf("parse agent config: %w", err)
		}
		cfg.PanelURL = strings.TrimSpace(af.PanelURL)
		cfg.Token = strings.TrimSpace(af.Token)
		cfg.RegisterToken = strings.TrimSpace(af.RegisterToken)
		cfg.ServerID = af.ServerID
		if af.StatePath != "" {
			cfg.StatePath = strings.TrimSpace(af.StatePath)
		}
		if af.LogLevel != "" {
			cfg.LogLevel = af.LogLevel
		}
		if af.HeartbeatIntervalSec > 0 {
			cfg.HeartbeatInterval = time.Duration(af.HeartbeatIntervalSec) * time.Second
		}
		if af.SyncIntervalSec > 0 {
			cfg.SyncInterval = time.Duration(af.SyncIntervalSec) * time.Second
		}
		if af.TrafficIntervalSec > 0 {
			cfg.TrafficInterval = time.Duration(af.TrafficIntervalSec) * time.Second
		}
		if af.SingBox.ConfigPath != "" {
			cfg.SingBox.ConfigPath = strings.TrimSpace(af.SingBox.ConfigPath)
		}
		if af.SingBox.CheckBin != "" {
			cfg.SingBox.CheckBin = strings.TrimSpace(af.SingBox.CheckBin)
		}
		cfg.SingBox.ReloadCommand = strings.TrimSpace(af.SingBox.ReloadCommand)
		if af.Collection.Traffic != nil {
			cfg.Collection.Traffic = *af.Collection.Traffic
		}
		if af.Collection.Sessions != nil {
			cfg.Collection.Sessions = *af.Collection.Sessions
		}
		if af.Collection.ConnectionLogs != nil {
			cfg.Collection.ConnectionLogs = *af.Collection.ConnectionLogs
		}
	}

	if v := os.Getenv("AGENT_PANEL_URL"); v != "" {
		cfg.PanelURL = v
	}
	if v := os.Getenv("AGENT_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("AGENT_REGISTER_TOKEN"); v != "" {
		cfg.RegisterToken = v
	}
	if v := os.Getenv("AGENT_SERVER_ID"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("AGENT_SERVER_ID: %w", err)
		}
		cfg.ServerID = n
	}
	if v := os.Getenv("AGENT_STATE_PATH"); v != "" {
		cfg.StatePath = v
	}
	if v := os.Getenv("AGENT_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("AGENT_HEARTBEAT_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.HeartbeatInterval = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("AGENT_SYNC_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.SyncInterval = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("AGENT_TRAFFIC_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.TrafficInterval = time.Duration(n) * time.Second
		}
	}
	if v := os.Getenv("AGENT_SINGBOX_CONFIG_PATH"); v != "" {
		cfg.SingBox.ConfigPath = v
	}
	if v := os.Getenv("AGENT_SINGBOX_CHECK_BIN"); v != "" {
		cfg.SingBox.CheckBin = v
	}
	if v := os.Getenv("AGENT_SINGBOX_RELOAD_COMMAND"); v != "" {
		cfg.SingBox.ReloadCommand = v
	}

	if cfg.PanelURL == "" {
		return nil, fmt.Errorf("panel_url is required")
	}
	if cfg.Token == "" && cfg.RegisterToken == "" {
		return nil, fmt.Errorf("token or register_token is required")
	}
	if cfg.ServerID < 1 {
		return nil, fmt.Errorf("server_id must be >= 1")
	}
	if cfg.StatePath == "" {
		return nil, fmt.Errorf("state_path is required")
	}
	if cfg.SingBox.ConfigPath == "" {
		return nil, fmt.Errorf("singbox.config_path is required")
	}
	if cfg.SingBox.CheckBin == "" {
		return nil, fmt.Errorf("singbox.check_bin is required")
	}
	if err := validateLogLevel(cfg.LogLevel); err != nil {
		return nil, err
	}
	for name, d := range map[string]time.Duration{
		"heartbeat_interval": cfg.HeartbeatInterval,
		"sync_interval":      cfg.SyncInterval,
		"traffic_interval":   cfg.TrafficInterval,
	} {
		if d < minAgentInterval {
			return nil, fmt.Errorf("%s must be >= %s", name, minAgentInterval)
		}
	}
	return cfg, nil
}

func decodeAppKey(hexStr string) ([]byte, error) {
	if hexStr == "" {
		return nil, fmt.Errorf("app_key must be %d-byte hex", appKeySize)
	}
	key, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("app_key must be hex: %w", err)
	}
	if len(key) != appKeySize {
		return nil, fmt.Errorf("app_key must decode to %d bytes, got %d", appKeySize, len(key))
	}
	return key, nil
}

func validateLogLevel(level string) error {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return fmt.Errorf("invalid log_level %q", level)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
