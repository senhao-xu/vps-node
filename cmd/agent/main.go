package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"vps-node/internal/agentclient"
	"vps-node/internal/agentruntime"
	"vps-node/internal/agentstate"
	"vps-node/internal/config"
	kernelsingbox "vps-node/internal/kernel/singbox"
	"vps-node/internal/logx"
)

const agentVersion = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agent:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "agent.yaml", "path to agent config file")
	showVersion := flag.Bool("version", false, "print agent version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("panel-agent " + agentVersion)
		return nil
	}

	if _, err := os.Stat(*configPath); err != nil {
		if flagPassed("config") {
			return fmt.Errorf("config file %q not found", *configPath)
		}
		*configPath = ""
	}

	cfg, err := config.LoadAgent(*configPath)
	if err != nil {
		return err
	}

	logger, err := logx.New(cfg.LogLevel)
	if err != nil {
		return err
	}

	state, err := agentstate.Load(cfg.StatePath)
	if err != nil {
		return err
	}

	client, err := agentclient.New(cfg.PanelURL)
	if err != nil {
		return err
	}

	kernel := kernelsingbox.NewRuntime()
	defer func() {
		if err := kernel.Stop(); err != nil {
			logger.Warn("stopping embedded sing-box failed", "error", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("agent starting",
		"panel_url", cfg.PanelURL,
		"state_path", cfg.StatePath,
		"heartbeat_interval", cfg.HeartbeatInterval.String(),
		"sync_interval", cfg.SyncInterval.String(),
		"traffic_interval", cfg.TrafficInterval.String(),
		"collection", map[string]bool{
			"traffic": cfg.Collection.Traffic,
		})

	loop := agentruntime.NewLoop(agentruntime.LoopOptions{
		Config:    cfg,
		Client:    client,
		State:     state,
		StatePath: cfg.StatePath,
		Kernel:    kernel,
		Metrics:   agentruntime.NewMetricsCollector(),
		Logger:    logger,
		Version:   agentVersion,
	})
	return loop.Run(ctx)
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
