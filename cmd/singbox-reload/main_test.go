package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

var fakeSingBoxBin string

const fakeSingBoxSrc = `package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "crash" {
		fmt.Fprintln(os.Stderr, "fake sing-box crashing")
		os.Exit(3)
	}
	fmt.Fprintln(os.Stderr, "fake sing-box up")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)
	<-sig
	fmt.Fprintln(os.Stderr, "fake sing-box terminated")
	os.Exit(0)
}
`

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "singbox-reload-test")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)

	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte(fakeSingBoxSrc), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin := filepath.Join(dir, "fake-sing-box")
	out, err := exec.Command("go", "build", "-o", bin, src).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build fake sing-box: %v\n%s", err, out)
		os.Exit(1)
	}
	fakeSingBoxBin = bin
	os.Exit(m.Run())
}

func writeConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "etc", "sing-box", "config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"inbounds":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func pidFromFile(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse pid file %q: %v", data, err)
	}
	return pid
}

func killAndWait(t *testing.T, pid int) {
	t.Helper()
	_ = syscall.Kill(pid, syscall.SIGTERM)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
}

func TestReloadStartsSingBoxWithoutPidFile(t *testing.T) {
	config := writeConfig(t)
	pidFile := filepath.Join(t.TempDir(), "run", "singbox.pid")
	opts := options{pidFile: pidFile, singBoxBin: fakeSingBoxBin, configPath: config}

	if err := run(opts); err != nil {
		t.Fatalf("run: %v", err)
	}
	pid := pidFromFile(t, pidFile)
	if !processAlive(pid) {
		t.Fatalf("started process %d is not alive", pid)
	}
	t.Cleanup(func() { killAndWait(t, pid) })
}

func TestReloadReplacesRunningSingBox(t *testing.T) {
	config := writeConfig(t)
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "singbox.pid")

	old := exec.Command(fakeSingBoxBin, "run", "-c", config)
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	oldPid := old.Process.Pid
	t.Cleanup(func() { _ = old.Wait() })

	if err := writePidFile(pidFile, oldPid); err != nil {
		t.Fatal(err)
	}
	opts := options{pidFile: pidFile, singBoxBin: fakeSingBoxBin, configPath: config}
	if err := run(opts); err != nil {
		t.Fatalf("run: %v", err)
	}
	if processAlive(oldPid) {
		t.Fatalf("old sing-box pid %d still alive", oldPid)
	}
	_ = old.Wait()

	newPid := pidFromFile(t, pidFile)
	if newPid == oldPid {
		t.Fatal("pid file was not updated with a new pid")
	}
	if !processAlive(newPid) {
		t.Fatalf("replacement sing-box pid %d is not alive", newPid)
	}
	t.Cleanup(func() { killAndWait(t, newPid) })
}

func TestReloadIgnoresStalePidFile(t *testing.T) {
	config := writeConfig(t)
	pidFile := filepath.Join(t.TempDir(), "singbox.pid")

	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	deadPid := dead.Process.Pid
	if err := writePidFile(pidFile, deadPid); err != nil {
		t.Fatal(err)
	}

	opts := options{pidFile: pidFile, singBoxBin: fakeSingBoxBin, configPath: config}
	if err := run(opts); err != nil {
		t.Fatalf("run: %v", err)
	}
	newPid := pidFromFile(t, pidFile)
	if newPid == deadPid {
		t.Fatal("pid file still holds the stale pid")
	}
	if !processAlive(newPid) {
		t.Fatalf("started sing-box pid %d is not alive", newPid)
	}
	t.Cleanup(func() { killAndWait(t, newPid) })
}

func TestReloadTreatsZombieAsDead(t *testing.T) {
	config := writeConfig(t)
	pidFile := filepath.Join(t.TempDir(), "singbox.pid")

	exited := exec.Command("true")
	if err := exited.Start(); err != nil {
		t.Fatal(err)
	}
	zombiePid := exited.Process.Pid
	t.Cleanup(func() { _ = exited.Wait() })

	deadline := time.Now().Add(5 * time.Second)
	for {
		state, err := procState(zombiePid)
		if err == nil && state == "Z" {
			break
		}
		if time.Now().After(deadline) {
			t.Skipf("pid %d never became a zombie (state=%v, err=%v)", zombiePid, state, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if processAlive(zombiePid) {
		t.Fatal("zombie reported as alive")
	}

	if err := writePidFile(pidFile, zombiePid); err != nil {
		t.Fatal(err)
	}
	opts := options{pidFile: pidFile, singBoxBin: fakeSingBoxBin, configPath: config}
	if err := run(opts); err != nil {
		t.Fatalf("run with zombie pid file: %v", err)
	}
	newPid := pidFromFile(t, pidFile)
	if !processAlive(newPid) {
		t.Fatalf("started sing-box pid %d is not alive", newPid)
	}
	t.Cleanup(func() { killAndWait(t, newPid) })
}

func TestReloadRefusesForeignProcess(t *testing.T) {
	config := writeConfig(t)
	pidFile := filepath.Join(t.TempDir(), "singbox.pid")

	foreign := exec.Command("sleep", "60")
	if err := foreign.Start(); err != nil {
		t.Fatal(err)
	}
	foreignPid := foreign.Process.Pid
	t.Cleanup(func() {
		_ = foreign.Process.Kill()
		_ = foreign.Wait()
	})
	if err := writePidFile(pidFile, foreignPid); err != nil {
		t.Fatal(err)
	}

	opts := options{pidFile: pidFile, singBoxBin: fakeSingBoxBin, configPath: config}
	err := run(opts)
	if err == nil {
		t.Fatal("expected refusal for foreign pid")
	}
	if !strings.Contains(err.Error(), "foreign process") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !processAlive(foreignPid) {
		t.Fatal("foreign process was killed")
	}
	if got := pidFromFile(t, pidFile); got != foreignPid {
		t.Fatal("pid file was modified on refusal")
	}
}

func TestReloadFailsWithoutConfig(t *testing.T) {
	opts := options{
		pidFile:    filepath.Join(t.TempDir(), "singbox.pid"),
		singBoxBin: fakeSingBoxBin,
		configPath: filepath.Join(t.TempDir(), "missing.json"),
	}
	if err := run(opts); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestReloadFailsWithoutBinary(t *testing.T) {
	opts := options{
		pidFile:    filepath.Join(t.TempDir(), "singbox.pid"),
		singBoxBin: filepath.Join(t.TempDir(), "missing-sing-box"),
		configPath: writeConfig(t),
	}
	if err := run(opts); err == nil {
		t.Fatal("expected error for missing binary")
	}
}
