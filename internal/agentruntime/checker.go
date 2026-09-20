package agentruntime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Checker interface {
	Check(path string) error
	Version(ctx context.Context) (string, error)
}

type CommandChecker struct {
	Bin  string
	Args []string
}

func NewSingBoxChecker(bin string) *CommandChecker {
	return &CommandChecker{Bin: bin, Args: []string{"check", "-c"}}
}

func (c *CommandChecker) run(ctx context.Context, timeout time.Duration, args ...string) (string, error) {
	if c.Bin == "" {
		return "", fmt.Errorf("checker binary is not configured")
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, c.Bin, args...)
	cmd.Env = minimalEnv()
	out, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("%s timed out after %s", c.Bin, timeout)
	}
	if err != nil {
		return string(out), fmt.Errorf("%s %s: %w: %s", c.Bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (c *CommandChecker) Check(path string) error {
	_, err := c.run(context.Background(), checkTimeout, append(append([]string{}, c.Args...), path)...)
	return err
}

func (c *CommandChecker) Version(ctx context.Context) (string, error) {
	out, err := c.run(ctx, checkTimeout, "version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

const checkTimeout = 30 * time.Second

func minimalEnv() []string {
	return []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"HOME=/root",
	}
}
