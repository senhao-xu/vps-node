package agentruntime

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"vps-node/internal/agentclient"
	"vps-node/internal/agentstate"
	"vps-node/internal/config"
)

const maxBatchRecords = 1000

type Loop struct {
	cfg       *config.Agent
	client    *agentclient.Client
	state     *agentstate.State
	statePath string
	applier   *Applier
	metrics   MetricsSource
	logger    *slog.Logger
	version   string

	mu             sync.Mutex
	collector      *Collector
	lastApplyError string
	forceApply     bool
}

type MetricsSource interface {
	Collect() (Metrics, error)
}

type LoopOptions struct {
	Config    *config.Agent
	Client    *agentclient.Client
	State     *agentstate.State
	StatePath string
	Applier   *Applier
	Metrics   MetricsSource
	Logger    *slog.Logger
	Version   string
}

func NewLoop(o LoopOptions) *Loop {
	if o.Logger == nil {
		o.Logger = discardLogger()
	}
	return &Loop{
		cfg:        o.Config,
		client:     o.Client,
		state:      o.State,
		statePath:  o.StatePath,
		applier:    o.Applier,
		metrics:    o.Metrics,
		logger:     o.Logger,
		version:    o.Version,
		forceApply: true,
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func (l *Loop) Run(ctx context.Context) error {
	if err := l.EnsureIdentity(ctx); err != nil {
		return err
	}
	l.logger.Info("agent identity ready",
		"agent_id", l.state.AgentID, "server_id", l.state.ServerID)

	syncNow := make(chan struct{}, 1)
	nudge := func() {
		select {
		case syncNow <- struct{}{}:
		default:
		}
	}

	l.heartbeatPass(ctx, nudge)
	l.syncPass(ctx)
	l.telemetryPass(ctx)

	hbTicker := time.NewTicker(l.cfg.HeartbeatInterval)
	defer hbTicker.Stop()
	syncTicker := time.NewTicker(l.cfg.SyncInterval)
	defer syncTicker.Stop()
	telemetryTicker := time.NewTicker(l.cfg.TrafficInterval)
	defer telemetryTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			l.logger.Info("agent stopped")
			return nil
		case <-hbTicker.C:
			interval := l.heartbeatPass(ctx, nudge)
			if interval > 0 && interval != l.cfg.HeartbeatInterval {
				l.cfg.HeartbeatInterval = interval
				hbTicker.Reset(interval)
			}
		case <-syncTicker.C:
			l.syncPass(ctx)
		case <-syncNow:
			l.syncPass(ctx)
		case <-telemetryTicker.C:
			l.telemetryPass(ctx)
		}
	}
}

func (l *Loop) EnsureIdentity(ctx context.Context) error {
	if l.cfg.Token != "" && l.state.AgentToken != "" && l.state.AgentToken != l.cfg.Token {
		l.logger.Info("config token changed; resetting stored agent identity")
		*l.state = agentstate.State{}
	}

	if l.state.ServerID != 0 && l.state.ServerID != l.cfg.ServerID {
		*l.state = agentstate.State{}
		l.logger.Warn("stored server_id does not match config; resetting state")
	}

	if l.state.AgentToken != "" || l.cfg.Token != "" {
		token := l.state.AgentToken
		if token == "" {
			token = l.cfg.Token
			l.state.AgentToken = token
		}
		l.client.SetToken(token)
		l.saveState()
		return nil
	}

	if l.cfg.RegisterToken == "" {
		return errors.New("no agent token available: set token or register_token in the agent config")
	}

	resp, err := l.client.Register(ctx, agentclient.RegisterRequest{
		RegisterToken: l.cfg.RegisterToken,
		Version:       l.version,
	})
	if err != nil {
		return err
	}
	if resp.ServerID != l.cfg.ServerID {
		return errors.New("register_token resolved to a different server; refusing to bind (check server_id in the agent config)")
	}
	l.state.AgentID = resp.AgentID
	l.state.AgentToken = resp.AgentToken
	l.state.ServerID = resp.ServerID
	l.state.AppliedRevision = 0
	l.state.TrafficBatchSeq = 0
	l.state.LogBatchSeq = 0
	l.client.SetToken(resp.AgentToken)
	l.saveState()
	l.logger.Info("agent registered", "agent_id", resp.AgentID, "server_id", resp.ServerID)
	return nil
}

func (l *Loop) saveState() {
	if err := agentstate.Save(l.statePath, l.state); err != nil {
		l.logger.Error("persist agent state failed", "error", err)
	}
}

func (l *Loop) heartbeatPass(ctx context.Context, nudge func()) time.Duration {
	if l.metrics == nil {
		return 0
	}
	m, err := l.metrics.Collect()
	if err != nil {
		l.logger.Warn("collect system metrics failed", "error", err)
		return 0
	}
	l.mu.Lock()
	lastErr := l.lastApplyError
	l.mu.Unlock()

	resp, err := l.client.Heartbeat(ctx, agentclient.HeartbeatRequest{
		Version:        l.version,
		CPUPercent:     m.CPUPercent,
		MemoryPercent:  m.MemoryPercent,
		DiskPercent:    m.DiskPercent,
		UptimeSeconds:  m.UptimeSeconds,
		LastApplyError: lastErr,
	})
	if err != nil {
		l.logger.Warn("heartbeat failed", "error", err)
		return 0
	}
	if resp.ServerRevision > l.state.AppliedRevision {
		nudge()
	}
	if resp.HeartbeatIntervalSeconds > 0 {
		return time.Duration(resp.HeartbeatIntervalSeconds) * time.Second
	}
	return 0
}

func (l *Loop) syncPass(ctx context.Context) {
	l.mu.Lock()
	force := l.forceApply
	l.mu.Unlock()

	applied := l.state.AppliedRevision
	if force {
		applied = 0
	}
	resp, err := l.client.Config(ctx, applied)
	if err != nil {
		l.logger.Warn("config poll failed; keeping last applied revision", "applied_revision", l.state.AppliedRevision, "error", err)
		return
	}
	if resp.Config == nil || len(resp.Config.Singbox) == 0 {
		return
	}
	if !force && resp.Status != "updated" {
		return
	}

	if err := l.applier.Apply(ctx, resp.Config.Singbox); err != nil {
		l.mu.Lock()
		l.lastApplyError = truncate(err.Error(), 256)
		l.mu.Unlock()
		l.logger.Error("apply sing-box config failed; previous config retained",
			"revision", resp.Revision, "error", err)
		return
	}

	l.state.AppliedRevision = resp.Revision
	l.saveState()
	l.mu.Lock()
	l.lastApplyError = ""
	l.forceApply = false
	l.mu.Unlock()
	l.logger.Info("sing-box config applied", "revision", resp.Revision, "renderer", resp.RendererVersion)

	if collector, err := l.buildCollector(resp); err != nil {
		l.logger.Warn("traffic/session collection unavailable for this config", "error", err)
	} else {
		l.mu.Lock()
		l.collector = collector
		l.mu.Unlock()
	}
}

func (l *Loop) buildCollector(resp *agentclient.ConfigResponse) (*Collector, error) {
	baseURL, secret, err := EndpointFromConfig(resp.Config.Singbox)
	if err != nil {
		return nil, err
	}
	source, err := NewClashClient(baseURL, secret)
	if err != nil {
		return nil, err
	}
	return NewCollector(source, BuildTable(resp.Users)), nil
}

func (l *Loop) telemetryPass(ctx context.Context) {
	l.mu.Lock()
	collector := l.collector
	l.mu.Unlock()
	if collector == nil {
		return
	}

	now := time.Now().UTC()
	res, err := collector.Poll(ctx, now)
	if err != nil {
		l.logger.Warn("collect runtime connections failed; skipping telemetry report", "error", err)
		return
	}
	if res.Unattributed > 0 {
		l.logger.Warn("connections without user attribution are excluded from reports",
			"unattributed", res.Unattributed, "attributed", res.Active)
	}

	if l.cfg.Collection.Sessions {
		l.reportSessions(ctx, res.Sessions, now)
	}
	if l.cfg.Collection.Traffic {
		l.reportTraffic(ctx, res.Traffic, now)
	}
	if l.cfg.Collection.ConnectionLogs {
		l.reportLogs(ctx, res.Closed, now)
	}
}

func (l *Loop) reportSessions(ctx context.Context, sessions []SessionSnapshot, now time.Time) {
	batch := agentclient.SessionBatch{
		ReportedAt: now.Format(time.RFC3339),
		Sessions:   make([]agentclient.SessionReport, 0, len(sessions)),
	}
	for _, s := range sessions {
		batch.Sessions = append(batch.Sessions, agentclient.SessionReport{
			UserID:        s.UserID,
			NodeID:        s.NodeID,
			IP:            s.IP,
			UploadBytes:   s.Upload,
			DownloadBytes: s.Download,
			ConnectedAt:   s.ConnectedAt.Format(time.RFC3339),
			LastSeenAt:    s.LastSeenAt.Format(time.RFC3339),
		})
	}
	ack, err := l.client.Sessions(ctx, batch)
	if err != nil {
		l.logger.Warn("session report failed", "error", err)
		return
	}
	l.logger.Debug("session snapshot reported", "sessions", ack.Sessions)
}

func (l *Loop) reportTraffic(ctx context.Context, deltas []TrafficDelta, now time.Time) {
	if len(deltas) == 0 {
		return
	}
	sort.Slice(deltas, func(i, j int) bool {
		if deltas[i].UserID != deltas[j].UserID {
			return deltas[i].UserID < deltas[j].UserID
		}
		return deltas[i].NodeID < deltas[j].NodeID
	})
	recordedAt := now.Format(time.RFC3339)
	for start := 0; start < len(deltas); start += maxBatchRecords {
		end := start + maxBatchRecords
		if end > len(deltas) {
			end = len(deltas)
		}
		chunk := deltas[start:end]
		batch := agentclient.TrafficBatch{
			BatchSeq: l.state.TrafficBatchSeq + 1,
			Records:  make([]agentclient.TrafficRecord, 0, len(chunk)),
		}
		for _, d := range chunk {
			batch.Records = append(batch.Records, agentclient.TrafficRecord{
				UserID:        d.UserID,
				NodeID:        d.NodeID,
				UploadBytes:   d.Upload,
				DownloadBytes: d.Download,
				RecordedAt:    recordedAt,
			})
		}
		ack, err := l.client.Traffic(ctx, batch)
		if err != nil {
			l.logger.Warn("traffic report failed; batch will be retried with the same sequence",
				"batch_seq", batch.BatchSeq, "error", err)
			return
		}
		l.state.TrafficBatchSeq = batch.BatchSeq
		l.saveState()
		l.logger.Info("traffic reported", "batch_seq", ack.BatchSeq, "records", ack.Records)
	}
}

func (l *Loop) reportLogs(ctx context.Context, closed []ClosedLog, now time.Time) {
	if len(closed) == 0 {
		return
	}
	sort.Slice(closed, func(i, j int) bool {
		if closed[i].UserID != closed[j].UserID {
			return closed[i].UserID < closed[j].UserID
		}
		return closed[i].NodeID < closed[j].NodeID
	})
	for start := 0; start < len(closed); start += maxBatchRecords {
		end := start + maxBatchRecords
		if end > len(closed) {
			end = len(closed)
		}
		chunk := closed[start:end]
		batch := agentclient.LogBatch{
			BatchSeq: l.state.LogBatchSeq + 1,
			Logs:     make([]agentclient.ConnectionLog, 0, len(chunk)),
		}
		for _, c := range chunk {
			closedAt := c.ClosedAt.Format(time.RFC3339)
			batch.Logs = append(batch.Logs, agentclient.ConnectionLog{
				UserID:        c.UserID,
				NodeID:        c.NodeID,
				IP:            c.IP,
				Protocol:      c.Protocol,
				UploadBytes:   c.Upload,
				DownloadBytes: c.Download,
				ConnectedAt:   c.ConnectedAt.Format(time.RFC3339),
				ClosedAt:      &closedAt,
				Status:        "closed",
			})
		}
		ack, err := l.client.ConnectionLogs(ctx, batch)
		if err != nil {
			l.logger.Warn("connection log report failed; batch will be retried with the same sequence",
				"batch_seq", batch.BatchSeq, "error", err)
			return
		}
		l.state.LogBatchSeq = batch.BatchSeq
		l.saveState()
		l.logger.Info("connection logs reported", "batch_seq", ack.BatchSeq, "logs", ack.Logs)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
