package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/config"
	"vps-node/internal/db"
	"vps-node/internal/httpx"
	"vps-node/internal/janitor"
	"vps-node/internal/logx"
	"vps-node/internal/repo"
	"vps-node/internal/web"
	"vps-node/internal/webui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "panel:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "panel.yaml", "path to panel config file")
	flag.Parse()

	if _, err := os.Stat(*configPath); err != nil {
		if flagPassed("config") {
			return fmt.Errorf("config file %q not found", *configPath)
		}
		*configPath = ""
	}

	cfg, err := config.LoadPanel(*configPath)
	if err != nil {
		return err
	}

	logger, err := logx.New(cfg.LogLevel)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return fmt.Errorf("create db dir: %w", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer database.Close()

	if err := database.Migrate(context.Background()); err != nil {
		return err
	}

	store := repo.New(database.DB)

	if err := cfg.ResolveAppKey(context.Background(),
		func(ctx context.Context) (string, error) {
			value, err := store.GetSetting(ctx, config.AppKeySettingKey)
			if errors.Is(err, repo.ErrNotFound) {
				return "", nil
			}
			return value, err
		},
		func(ctx context.Context, value string) error {
			return store.SetSetting(ctx, config.AppKeySettingKey, value)
		},
	); err != nil {
		return err
	}
	logger.Info("app key resolved", "source", cfg.AppKeySource())

	sessions := adminauth.NewSessions(database.DB, 24*time.Hour, cfg.SecureCookie)
	limiter := adminauth.NewLimiter(5, 15*time.Minute, 15*time.Minute)

	handler, err := web.New(web.Options{
		DB:            database.DB,
		Repo:          store,
		Sessions:      sessions,
		Limiter:       limiter,
		AppKey:        cfg.AppKey(),
		Logger:        logger,
		AdminUsername: cfg.Admin.Username,
		AdminPassword: cfg.Admin.Password,
	})
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/", handler)

	root := webui.Wrap(mux, logger)
	srv := httpx.NewServer(httpx.ServerOptions{Addr: cfg.Listen, Handler: root, Logger: logger})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sweeper := janitor.New(store, cfg.Retention.SweepInterval, logger).
		WithCaps(cfg.Retention.MaxConnectionLogs, cfg.Retention.MaxTrafficRecords)
	go sweeper.Run(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	logger.Info("panel started", "addr", cfg.Listen, "db", cfg.DBPath,
		"retention_raw_log_days", cfg.Retention.RawLogDays,
		"retention_aggregate_days", cfg.Retention.AggregateDays)

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	logger.Info("panel stopped")
	return nil
}

func flagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
