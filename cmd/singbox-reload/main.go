package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	terminateWaitTimeout = 10 * time.Second
	killWaitTimeout      = 3 * time.Second
	pollInterval         = 100 * time.Millisecond
)

type options struct {
	pidFile    string
	singBoxBin string
	configPath string
}

func optionsFromEnv() options {
	return options{
		pidFile:    envOr("SINGBOX_PIDFILE", "/run/singbox/singbox.pid"),
		singBoxBin: envOr("SINGBOX_BIN", "/usr/local/bin/sing-box"),
		configPath: envOr("SINGBOX_CONFIG", "/etc/sing-box/config.json"),
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func main() {
	if err := run(optionsFromEnv()); err != nil {
		fmt.Fprintln(os.Stderr, "singbox-reload:", err)
		os.Exit(1)
	}
}

func run(o options) error {
	if _, err := os.Stat(o.configPath); err != nil {
		return fmt.Errorf("config %s not readable: %w", o.configPath, err)
	}
	if _, err := os.Stat(o.singBoxBin); err != nil {
		return fmt.Errorf("sing-box binary %s not found: %w", o.singBoxBin, err)
	}

	pid, alive, err := findRunningSingBox(o)
	if err != nil {
		return err
	}
	if alive {
		if err := terminate(pid); err != nil {
			return fmt.Errorf("stopping previous sing-box (pid %d): %w", pid, err)
		}
	}

	cmd := exec.Command(o.singBoxBin, "run", "-c", o.configPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s run: %w", o.singBoxBin, err)
	}
	if err := writePidFile(o.pidFile, cmd.Process.Pid); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("write pid file %s: %w", o.pidFile, err)
	}
	fmt.Fprintf(os.Stdout, "singbox-reload: sing-box running (pid %d, config %s)\n", cmd.Process.Pid, o.configPath)
	return nil
}

func findRunningSingBox(o options) (int, bool, error) {
	data, err := os.ReadFile(o.pidFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read pid file %s: %w", o.pidFile, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid < 1 {
		return 0, false, nil
	}
	if !processAlive(pid) {
		return 0, false, nil
	}
	if !isSameBinary(pid, o.singBoxBin) {
		return pid, false, fmt.Errorf("pid file %s holds pid %d of a foreign process; refusing to signal it (delete the file if stale)", o.pidFile, pid)
	}
	return pid, true, nil
}

func terminate(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return fmt.Errorf("send SIGTERM: %w", err)
	}
	if !waitForExit(pid, terminateWaitTimeout) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		if !waitForExit(pid, killWaitTimeout) {
			return fmt.Errorf("pid %d still running after SIGKILL", pid)
		}
	}
	return nil
}

func waitForExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !processAlive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(pollInterval)
	}
}

func processAlive(pid int) bool {
	state, err := procState(pid)
	if err != nil {
		return syscall.Kill(pid, 0) == nil
	}
	return state != "Z"
}

func procState(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	s := string(data)
	idx := strings.LastIndex(s, ")")
	if idx < 0 || idx+2 >= len(s) {
		return "", fmt.Errorf("malformed stat for pid %d", pid)
	}
	return strings.TrimSpace(s[idx+2 : idx+3]), nil
}

func isSameBinary(pid int, bin string) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		return false
	}
	fields := strings.Split(string(data), "\x00")
	if len(fields) == 0 || fields[0] == "" {
		return false
	}
	if fields[0] == bin {
		return true
	}
	return filepath.Base(fields[0]) == filepath.Base(bin)
}

func writePidFile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
