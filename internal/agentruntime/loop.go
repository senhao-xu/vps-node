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
	kernelsingbox "vps-node/internal/kernel/singbox"
)

const (
	maxBatchRecords  = 1000
	maxPendingVisits = 10000
)

type Kernel interface {
	Start(configJSON []byte, users []kernelsingbox.UserRef) error
	Stop() error
	Snapshot() kernelsingbox.Snapshot
	DrainVisits() []kernelsingbox.Visit
}

type Loop struct {
	cfg       *config.Agent
	client    *agentclient.Client
	state     *agentstate.State
	statePath string
	kernel    Kernel
	metrics   MetricsSource
	logger    *slog.Logger
	version   string

	mu                sync.Mutex
	lastSeen          map[kernelsingbox.Pair]kernelsingbox.Traffic
	pending           map[kernelsingbox.Pair]kernelsingbox.Traffic
	inflight          []agentclient.TrafficBatch
	inflightIdx       int
	deviceInflight    []agentclient.DeviceBatch
	deviceInflightIdx int
	visitInflight     []agentclient.VisitBatch
	visitInflightIdx  int
	pendingVisits     []kernelsingbox.Visit
	lastApplyError    string
	forceApply        bool
}

type MetricsSource interface {
	Collect() (Metrics, error)
}

type LoopOptions struct {
	Config    *config.Agent
	Client    *agentclient.Client
	State     *agentstate.State
	StatePath string
	Kernel    Kernel
	Metrics   MetricsSource
	Logger    *slog.Logger
	Version   string
}

func NewLoop(o LoopOptions) *Loop {
	if o.Logger == nil {
		o.Logger = discardLogger()
	}
	if o.Config == nil {
		o.Config = &config.Agent{}
	}
	return &Loop{
		cfg:        o.Config,
		client:     o.Client,
		state:      o.State,
		statePath:  o.StatePath,
		kernel:     o.Kernel,
		metrics:    o.Metrics,
		logger:     o.Logger,
		version:    o.Version,
		lastSeen:   map[kernelsingbox.Pair]kernelsingbox.Traffic{},
		pending:    map[kernelsingbox.Pair]kernelsingbox.Traffic{},
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
			if l.kernel != nil {
				_ = l.kernel.Stop()
			}
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
	l.state.DeviceBatchSeq = 0
	l.state.VisitBatchSeq = 0
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

func (l *Loop) SyncOnce(ctx context.Context) {
	l.syncPass(ctx)
}

func (l *Loop) TelemetryOnce(ctx context.Context) {
	l.telemetryPass(ctx)
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
	if l.kernel == nil {
		return
	}

	l.collectDeltas(l.kernel.Snapshot().Traffic)
	if err := l.kernel.Start(resp.Config.Singbox, toUserRefs(resp.Users)); err != nil {
		l.mu.Lock()
		l.lastApplyError = truncate(err.Error(), 256)
		l.mu.Unlock()
		l.logger.Error("start embedded sing-box failed; will retry on the next sync",
			"revision", resp.Revision, "error", err)
		return
	}

	l.mu.Lock()
	l.lastSeen = map[kernelsingbox.Pair]kernelsingbox.Traffic{}
	l.lastApplyError = ""
	l.forceApply = false
	l.mu.Unlock()
	l.state.AppliedRevision = resp.Revision
	l.saveState()
	l.logger.Info("sing-box config applied", "revision", resp.Revision, "renderer", resp.RendererVersion)
}

func (l *Loop) telemetryPass(ctx context.Context) {
	if l.kernel == nil {
		return
	}
	visits := l.kernel.DrainVisits()
	snapshot := l.kernel.Snapshot()
	now := time.Now().UTC()
	l.collectDeltas(snapshot.Traffic)
	if l.cfg.Collection.Traffic {
		l.flushTraffic(ctx, now)
	}
	l.flushDevices(ctx, snapshot.Devices, now)
	if l.cfg.Collection.Visits {
		l.flushVisits(ctx, visits)
	}
}

func (l *Loop) collectDeltas(current map[kernelsingbox.Pair]kernelsingbox.Traffic) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for pair, total := range current {
		prev := l.lastSeen[pair]
		upload := total.Upload - prev.Upload
		download := total.Download - prev.Download
		if upload < 0 {
			upload = 0
		}
		if download < 0 {
			download = 0
		}
		if upload != 0 || download != 0 {
			pending := l.pending[pair]
			pending.Upload += upload
			pending.Download += download
			l.pending[pair] = pending
		}
		l.lastSeen[pair] = total
	}
}

func (l *Loop) flushTraffic(ctx context.Context, now time.Time) {
	l.mu.Lock()
	if len(l.inflight) == 0 {
		if len(l.pending) == 0 {
			l.mu.Unlock()
			return
		}
		l.inflight = buildTrafficBatches(l.pending, l.state.TrafficBatchSeq, now)
		l.inflightIdx = 0
		l.pending = map[kernelsingbox.Pair]kernelsingbox.Traffic{}
	}
	batches := l.inflight
	idx := l.inflightIdx
	l.mu.Unlock()

	for idx < len(batches) {
		ack, err := l.client.Traffic(ctx, batches[idx])
		if err != nil {
			if !agentclient.Retryable(err) {
				l.requeueTrafficBatch(batches[idx])
				l.logger.Warn("traffic batch permanently rejected; deltas requeued under a fresh sequence",
					"batch_seq", batches[idx].BatchSeq, "error", err)
				idx++
				continue
			}
			l.mu.Lock()
			l.inflightIdx = idx
			l.mu.Unlock()
			l.logger.Warn("traffic report failed; frozen batch will be replayed with the same payload",
				"batch_seq", batches[idx].BatchSeq, "error", err)
			return
		}
		idx++
		l.mu.Lock()
		l.state.TrafficBatchSeq = batches[idx-1].BatchSeq
		l.inflightIdx = idx
		l.mu.Unlock()
		l.saveState()
		l.logger.Info("traffic reported", "batch_seq", ack.BatchSeq, "records", ack.Records)
	}

	l.mu.Lock()
	l.inflight = nil
	l.inflightIdx = 0
	l.mu.Unlock()
}

func (l *Loop) requeueTrafficBatch(batch agentclient.TrafficBatch) {
	l.mu.Lock()
	for _, rec := range batch.Records {
		pair := kernelsingbox.Pair{UserID: rec.UserID, NodeID: rec.NodeID}
		pending := l.pending[pair]
		pending.Upload += rec.U
		pending.Download += rec.D
		l.pending[pair] = pending
	}
	if batch.BatchSeq > l.state.TrafficBatchSeq {
		l.state.TrafficBatchSeq = batch.BatchSeq
	}
	l.mu.Unlock()
	l.saveState()
}

func buildTrafficBatches(pending map[kernelsingbox.Pair]kernelsingbox.Traffic, base int64, now time.Time) []agentclient.TrafficBatch {
	records := make([]agentclient.TrafficRecord, 0, len(pending))
	recordedAt := now.Format(time.RFC3339)
	for pair, total := range pending {
		records = append(records, agentclient.TrafficRecord{
			UserID:     pair.UserID,
			NodeID:     pair.NodeID,
			U:          total.Upload,
			D:          total.Download,
			RecordedAt: recordedAt,
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].UserID != records[j].UserID {
			return records[i].UserID < records[j].UserID
		}
		return records[i].NodeID < records[j].NodeID
	})

	batches := make([]agentclient.TrafficBatch, 0, (len(records)+maxBatchRecords-1)/maxBatchRecords)
	for start := 0; start < len(records); start += maxBatchRecords {
		end := start + maxBatchRecords
		if end > len(records) {
			end = len(records)
		}
		batches = append(batches, agentclient.TrafficBatch{
			BatchSeq: base + int64(len(batches)) + 1,
			Records:  records[start:end],
		})
	}
	return batches
}

func (l *Loop) flushDevices(ctx context.Context, devices []kernelsingbox.Device, now time.Time) {
	l.mu.Lock()
	if len(l.deviceInflight) == 0 {
		l.deviceInflight = buildDeviceBatches(devices, l.state.DeviceBatchSeq, now)
		l.deviceInflightIdx = 0
	}
	batches := l.deviceInflight
	idx := l.deviceInflightIdx
	l.mu.Unlock()

	for idx < len(batches) {
		ack, err := l.client.Devices(ctx, batches[idx])
		if err != nil {
			if !agentclient.Retryable(err) {
				l.mu.Lock()
				if batches[idx].BatchSeq > l.state.DeviceBatchSeq {
					l.state.DeviceBatchSeq = batches[idx].BatchSeq
				}
				l.deviceInflight = nil
				l.deviceInflightIdx = 0
				l.mu.Unlock()
				l.saveState()
				l.logger.Warn("device snapshot permanently rejected; the latest snapshot will be resent",
					"batch_seq", batches[idx].BatchSeq, "error", err)
				return
			}
			l.mu.Lock()
			l.deviceInflightIdx = idx
			l.mu.Unlock()
			l.logger.Warn("device report failed; frozen batch will be replayed with the same payload",
				"batch_seq", batches[idx].BatchSeq, "error", err)
			return
		}
		idx++
		l.mu.Lock()
		l.state.DeviceBatchSeq = batches[idx-1].BatchSeq
		l.deviceInflightIdx = idx
		l.mu.Unlock()
		l.saveState()
		l.logger.Info("devices reported", "batch_seq", ack.BatchSeq, "devices", ack.Devices)
	}

	l.mu.Lock()
	l.deviceInflight = nil
	l.deviceInflightIdx = 0
	l.mu.Unlock()
}

func buildDeviceBatches(devices []kernelsingbox.Device, base int64, now time.Time) []agentclient.DeviceBatch {
	reports := make([]agentclient.DeviceReport, 0, len(devices))
	recordedAt := now.Format(time.RFC3339)
	for _, d := range devices {
		ips := append([]string{}, d.IPs...)
		sort.Strings(ips)
		reports = append(reports, agentclient.DeviceReport{
			UserID: d.UserID,
			NodeID: d.NodeID,
			IPs:    ips,
			Online: d.Online,
		})
	}
	sort.Slice(reports, func(i, j int) bool {
		if reports[i].UserID != reports[j].UserID {
			return reports[i].UserID < reports[j].UserID
		}
		return reports[i].NodeID < reports[j].NodeID
	})

	batches := make([]agentclient.DeviceBatch, 0, (len(reports)+maxBatchRecords-1)/maxBatchRecords)
	if len(reports) == 0 {
		return append(batches, agentclient.DeviceBatch{
			BatchSeq:   base + 1,
			RecordedAt: recordedAt,
			Devices:    []agentclient.DeviceReport{},
		})
	}
	for start := 0; start < len(reports); start += maxBatchRecords {
		end := start + maxBatchRecords
		if end > len(reports) {
			end = len(reports)
		}
		batches = append(batches, agentclient.DeviceBatch{
			BatchSeq:   base + int64(len(batches)) + 1,
			RecordedAt: recordedAt,
			Devices:    reports[start:end],
		})
	}
	return batches
}

func (l *Loop) flushVisits(ctx context.Context, visits []kernelsingbox.Visit) {
	l.mu.Lock()
	if len(visits) > 0 {
		l.pendingVisits = append(l.pendingVisits, visits...)
		if excess := len(l.pendingVisits) - maxPendingVisits; excess > 0 {
			l.pendingVisits = l.pendingVisits[excess:]
			l.logger.Warn("visit backlog overflow; dropping oldest pending visits", "dropped", excess)
		}
	}
	if len(l.visitInflight) == 0 {
		if len(l.pendingVisits) == 0 {
			l.mu.Unlock()
			return
		}
		l.visitInflight = buildVisitBatches(l.pendingVisits, l.state.VisitBatchSeq)
		l.pendingVisits = nil
		l.visitInflightIdx = 0
	}
	batches := l.visitInflight
	idx := l.visitInflightIdx
	l.mu.Unlock()

	for idx < len(batches) {
		ack, err := l.client.Visits(ctx, batches[idx])
		if err != nil {
			if !agentclient.Retryable(err) {
				l.mu.Lock()
				if batches[idx].BatchSeq > l.state.VisitBatchSeq {
					l.state.VisitBatchSeq = batches[idx].BatchSeq
				}
				l.visitInflight = nil
				l.visitInflightIdx = 0
				l.mu.Unlock()
				l.saveState()
				l.logger.Warn("visit batch permanently rejected; new visits will be reported under a fresh sequence",
					"batch_seq", batches[idx].BatchSeq, "error", err)
				return
			}
			l.mu.Lock()
			l.visitInflightIdx = idx
			l.mu.Unlock()
			l.logger.Warn("visit report failed; frozen batch will be replayed with the same payload",
				"batch_seq", batches[idx].BatchSeq, "error", err)
			return
		}
		idx++
		l.mu.Lock()
		l.state.VisitBatchSeq = batches[idx-1].BatchSeq
		l.visitInflightIdx = idx
		l.mu.Unlock()
		l.saveState()
		l.logger.Info("visits reported", "batch_seq", ack.BatchSeq, "records", ack.Records)
	}

	l.mu.Lock()
	l.visitInflight = nil
	l.visitInflightIdx = 0
	l.mu.Unlock()
}

func buildVisitBatches(visits []kernelsingbox.Visit, base int64) []agentclient.VisitBatch {
	if len(visits) == 0 {
		return nil
	}
	records := make([]agentclient.VisitRecord, 0, len(visits))
	for _, v := range visits {
		records = append(records, agentclient.VisitRecord{
			UserID:     v.UserID,
			NodeID:     v.NodeID,
			DestHost:   v.DestHost,
			DestPort:   v.DestPort,
			Network:    v.Network,
			ClientIP:   v.ClientIP,
			RecordedAt: v.At.UTC().Format(time.RFC3339),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].UserID != records[j].UserID {
			return records[i].UserID < records[j].UserID
		}
		if records[i].NodeID != records[j].NodeID {
			return records[i].NodeID < records[j].NodeID
		}
		if records[i].DestHost != records[j].DestHost {
			return records[i].DestHost < records[j].DestHost
		}
		if records[i].DestPort != records[j].DestPort {
			return records[i].DestPort < records[j].DestPort
		}
		return records[i].RecordedAt < records[j].RecordedAt
	})

	batches := make([]agentclient.VisitBatch, 0, (len(records)+maxBatchRecords-1)/maxBatchRecords)
	for start := 0; start < len(records); start += maxBatchRecords {
		end := start + maxBatchRecords
		if end > len(records) {
			end = len(records)
		}
		batches = append(batches, agentclient.VisitBatch{
			BatchSeq: base + int64(len(batches)) + 1,
			Records:  records[start:end],
		})
	}
	return batches
}

func toUserRefs(users []agentclient.User) []kernelsingbox.UserRef {
	out := make([]kernelsingbox.UserRef, 0, len(users))
	for _, u := range users {
		ref := kernelsingbox.UserRef{ID: u.ID, DeviceLimit: u.DeviceLimit, Nodes: make([]kernelsingbox.NodeRef, 0, len(u.Nodes))}
		for _, n := range u.Nodes {
			ref.Nodes = append(ref.Nodes, kernelsingbox.NodeRef{
				ID:       n.ID,
				Protocol: n.Protocol,
				Port:     n.Port,
			})
		}
		out = append(out, ref)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
