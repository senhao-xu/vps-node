package agentruntime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	reloadTimeout     = 30 * time.Second
	tempFilePattern   = ".singbox-apply-*.json"
	secretFileMode    = 0o600
	directoryFileMode = 0o755
)

type ReloadCommand struct {
	Bin  string
	Args []string
}

func ParseReloadCommand(raw string) (*ReloadCommand, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	fields := strings.Fields(raw)
	for _, f := range fields {
		if strings.ContainsAny(f, ";|&$`'\"\\\n") {
			return nil, fmt.Errorf("reload_command must be a single binary with fixed arguments, got %q", raw)
		}
	}
	return &ReloadCommand{Bin: fields[0], Args: fields[1:]}, nil
}

type Applier struct {
	configPath string
	checker    Checker
	reload     *ReloadCommand
	logger     *slog.Logger
}

func NewApplier(configPath string, checker Checker, reload *ReloadCommand, logger *slog.Logger) *Applier {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Applier{configPath: configPath, checker: checker, reload: reload, logger: logger}
}

func (a *Applier) ConfigPath() string {
	return a.configPath
}

func (a *Applier) Apply(ctx context.Context, configJSON []byte) error {
	dir := filepath.Dir(a.configPath)
	if err := os.MkdirAll(dir, directoryFileMode); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, tempFilePattern)
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(secretFileMode); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp config: %w", err)
	}
	if _, err := tmp.Write(configJSON); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}

	if a.checker != nil {
		if err := a.checker.Check(tmpName); err != nil {
			return fmt.Errorf("sing-box check rejected config: %w", err)
		}
	}

	if err := os.Rename(tmpName, a.configPath); err != nil {
		return fmt.Errorf("activate config: %w", err)
	}

	if a.reload == nil {
		a.logger.Info("sing-box config activated (no reload command configured; restart sing-box manually or set singbox.reload_command)",
			"path", a.configPath)
		return nil
	}
	if err := a.runReload(ctx); err != nil {
		return fmt.Errorf("config file updated at %s but reload failed (previous process still running old config): %w", a.configPath, err)
	}
	a.logger.Info("sing-box config applied and reloaded", "path", a.configPath)
	return nil
}

func (a *Applier) runReload(ctx context.Context) error {
	runCtx, cancel := context.WithTimeout(ctx, reloadTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, a.reload.Bin, a.reload.Args...)
	out, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("reload command timed out after %s", reloadTimeout)
	}
	if err != nil {
		return fmt.Errorf("reload command %s: %w: %s", a.reload.Bin, err, strings.TrimSpace(string(out)))
	}
	return nil
}
