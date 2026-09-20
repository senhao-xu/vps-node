package logx

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func New(level string) (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.TrimSpace(level))); err != nil {
		return nil, fmt.Errorf("invalid log level %q: %w", level, err)
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})), nil
}
