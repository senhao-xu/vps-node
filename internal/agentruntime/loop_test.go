package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"vps-node/internal/agentclient"
	"vps-node/internal/agentstate"
	"vps-node/internal/config"
	kernelsingbox "vps-node/internal/kernel/singbox"
)

type stubKernel struct {
	mu        sync.Mutex
	starts    [][]byte
	users     [][]kernelsingbox.UserRef
	failTimes int
	snapshot  kernelsingbox.Snapshot
	visits    []kernelsingbox.Visit
}

func (k *stubKernel) Start(configJSON []byte, users []kernelsingbox.UserRef) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.failTimes > 0 {
		k.failTimes--
		return errors.New("stub start failure")
	}
	k.starts = append(k.starts, append([]byte(nil), configJSON...))
	k.users = append(k.users, users)
	return nil
}

func (k *stubKernel) Stop() error { return nil }

func (k *stubKernel) Snapshot() kernelsingbox.Snapshot {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.snapshot.Traffic == nil {
		return kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{}}
	}
	return k.snapshot
}

func (k *stubKernel) setSnapshot(snap kernelsingbox.Snapshot) {
	k.mu.Lock()
	k.snapshot = snap
	k.mu.Unlock()
}

func (k *stubKernel) DrainVisits() []kernelsingbox.Visit {
	k.mu.Lock()
	defer k.mu.Unlock()
	visits := k.visits
	k.visits = nil
	return visits
}

func (k *stubKernel) setVisits(visits []kernelsingbox.Visit) {
	k.mu.Lock()
	k.visits = visits
	k.mu.Unlock()
}

func (k *stubKernel) startCount() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.starts)
}

func newTestLoop(t *testing.T, client *agentclient.Client, kernel Kernel, state *agentstate.State) *Loop {
	t.Helper()
	if state == nil {
		state = &agentstate.State{}
	}
	return NewLoop(LoopOptions{
		Config:    &config.Agent{Collection: config.Collection{Traffic: true}},
		Client:    client,
		State:     state,
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Kernel:    kernel,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestSyncPassForcesApplyOnStartup(t *testing.T) {
	const revision = 5
	var mu sync.Mutex
	var gotVersions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/config" {
			http.NotFound(w, r)
			return
		}
		version := r.URL.Query().Get("version")
		mu.Lock()
		gotVersions = append(gotVersions, version)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if version == strconv.Itoa(revision) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "current", "revision": revision})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "updated",
			"revision": revision,
			"config":   map[string]any{"singbox": json.RawMessage(`{"inbounds":[]}`)},
		})
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{}
	state := &agentstate.State{AppliedRevision: revision}
	loop := newTestLoop(t, client, kernel, state)

	loop.syncPass(context.Background())

	mu.Lock()
	versions := append([]string(nil), gotVersions...)
	mu.Unlock()
	if len(versions) != 1 || versions[0] != "" {
		t.Fatalf("startup sync must request the full config (no version), got %v", versions)
	}
	if kernel.startCount() != 1 {
		t.Fatalf("startup sync must start the kernel once, got %d", kernel.startCount())
	}
	if got := string(kernel.starts[0]); got != `{"inbounds":[]}` {
		t.Fatalf("unexpected applied config: %s", got)
	}
	if state.AppliedRevision != revision {
		t.Fatalf("applied revision = %d, want %d", state.AppliedRevision, revision)
	}

	loop.syncPass(context.Background())

	mu.Lock()
	versions = append([]string(nil), gotVersions...)
	mu.Unlock()
	if len(versions) != 2 || versions[1] != strconv.Itoa(revision) {
		t.Fatalf("steady-state sync must send the applied revision, got %v", versions)
	}
	if kernel.startCount() != 1 {
		t.Fatalf("current revision must not restart the kernel, got %d starts", kernel.startCount())
	}
}

func TestSyncPassRetriesForceUntilApplySucceeds(t *testing.T) {
	const revision = 3
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("version") == strconv.Itoa(revision) {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "current", "revision": revision})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":   "updated",
			"revision": revision,
			"config":   map[string]any{"singbox": json.RawMessage(`{"inbounds":[]}`)},
		})
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{failTimes: 1}
	state := &agentstate.State{AppliedRevision: revision}
	loop := newTestLoop(t, client, kernel, state)

	loop.syncPass(context.Background())
	if !loop.forceApply {
		t.Fatal("failed startup apply must keep forcing on subsequent syncs")
	}
	if kernel.startCount() != 0 {
		t.Fatalf("expected no successful starts, got %d", kernel.startCount())
	}

	loop.syncPass(context.Background())
	if loop.forceApply {
		t.Fatal("successful apply must clear the force flag")
	}
	if state.AppliedRevision != revision {
		t.Fatalf("applied revision = %d, want %d", state.AppliedRevision, revision)
	}
	if kernel.startCount() != 1 {
		t.Fatalf("expected 1 successful start, got %d", kernel.startCount())
	}
}

func TestTelemetryDeltasAndReplay(t *testing.T) {
	var mu sync.Mutex
	var batches []agentclient.TrafficBatch
	failTraffic := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/traffic":
			mu.Lock()
			fail := failTraffic
			mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"boom"}}`))
				return
			}
			var batch agentclient.TrafficBatch
			_ = json.NewDecoder(r.Body).Decode(&batch)
			mu.Lock()
			batches = append(batches, batch)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(agentclient.TrafficAck{Accepted: true, BatchSeq: batch.BatchSeq, Records: int64(len(batch.Records))})
		case "/api/agent/devices":
			var batch agentclient.DeviceBatch
			_ = json.NewDecoder(r.Body).Decode(&batch)
			_ = json.NewEncoder(w).Encode(agentclient.DeviceAck{Accepted: true, BatchSeq: batch.BatchSeq, Devices: int64(len(batch.Devices))})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{}
	state := &agentstate.State{}
	loop := newTestLoop(t, client, kernel, state)
	pair := kernelsingbox.Pair{UserID: 1, NodeID: 7}

	kernel.setSnapshot(kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{
		pair: {Upload: 100, Download: 200},
	}})
	loop.telemetryPass(context.Background())

	mu.Lock()
	if len(batches) != 1 || batches[0].BatchSeq != 1 {
		mu.Unlock()
		t.Fatalf("expected batch seq 1, got %+v", batches)
	}
	if len(batches[0].Records) != 1 || batches[0].Records[0].U != 100 || batches[0].Records[0].D != 200 {
		mu.Unlock()
		t.Fatalf("unexpected first batch records %+v", batches[0].Records)
	}
	mu.Unlock()
	if state.TrafficBatchSeq != 1 {
		t.Fatalf("batch seq not persisted: %d", state.TrafficBatchSeq)
	}

	kernel.setSnapshot(kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{
		pair: {Upload: 150, Download: 260},
	}})
	loop.telemetryPass(context.Background())
	mu.Lock()
	if len(batches) != 2 || batches[1].BatchSeq != 2 {
		mu.Unlock()
		t.Fatalf("expected batch seq 2, got %+v", batches)
	}
	if batches[1].Records[0].U != 50 || batches[1].Records[0].D != 60 {
		mu.Unlock()
		t.Fatalf("expected deltas 50/60, got %+v", batches[1].Records[0])
	}
	mu.Unlock()

	mu.Lock()
	failTraffic = true
	mu.Unlock()
	kernel.setSnapshot(kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{
		pair: {Upload: 175, Download: 300},
	}})
	loop.telemetryPass(context.Background())
	if state.TrafficBatchSeq != 2 {
		t.Fatalf("failed report must not advance the batch seq, got %d", state.TrafficBatchSeq)
	}

	mu.Lock()
	failTraffic = false
	mu.Unlock()
	loop.telemetryPass(context.Background())
	mu.Lock()
	last := batches[len(batches)-1]
	mu.Unlock()
	if last.BatchSeq != 3 {
		t.Fatalf("replayed batch must reuse seq 3, got %d", last.BatchSeq)
	}
	if last.Records[0].U != 25 || last.Records[0].D != 40 {
		t.Fatalf("expected replayed deltas 25/40, got %+v", last.Records[0])
	}
}

func TestTelemetryReplayKeepsFrozenBatch(t *testing.T) {
	var mu sync.Mutex
	applied := map[int64][]agentclient.TrafficRecord{}
	var sent []agentclient.TrafficBatch
	failTraffic := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/traffic":
			var batch agentclient.TrafficBatch
			_ = json.NewDecoder(r.Body).Decode(&batch)
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, batch)
			if failTraffic {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"boom"}}`))
				return
			}
			if prior, duplicate := applied[batch.BatchSeq]; duplicate {
				_ = json.NewEncoder(w).Encode(agentclient.TrafficAck{Accepted: true, BatchSeq: batch.BatchSeq, Records: int64(len(prior))})
				return
			}
			applied[batch.BatchSeq] = batch.Records
			_ = json.NewEncoder(w).Encode(agentclient.TrafficAck{Accepted: true, BatchSeq: batch.BatchSeq, Records: int64(len(batch.Records))})
		case "/api/agent/devices":
			var batch agentclient.DeviceBatch
			_ = json.NewDecoder(r.Body).Decode(&batch)
			_ = json.NewEncoder(w).Encode(agentclient.DeviceAck{Accepted: true, BatchSeq: batch.BatchSeq, Devices: int64(len(batch.Devices))})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{}
	state := &agentstate.State{}
	loop := newTestLoop(t, client, kernel, state)
	pair := kernelsingbox.Pair{UserID: 1, NodeID: 7}
	ctx := context.Background()

	kernel.setSnapshot(kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{pair: {Upload: 100, Download: 200}}})
	loop.telemetryPass(ctx)

	mu.Lock()
	failTraffic = true
	mu.Unlock()
	kernel.setSnapshot(kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{pair: {Upload: 150, Download: 260}}})
	loop.telemetryPass(ctx)

	kernel.setSnapshot(kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{pair: {Upload: 200, Download: 400}}})
	loop.telemetryPass(ctx)

	mu.Lock()
	failTraffic = false
	mu.Unlock()
	loop.telemetryPass(ctx)
	loop.telemetryPass(ctx)

	mu.Lock()
	defer mu.Unlock()
	var up, down int64
	for _, records := range applied {
		for _, record := range records {
			up += record.U
			down += record.D
		}
	}
	if up != 200 || down != 400 {
		t.Fatalf("replay lost deltas: applied upload=%d download=%d, want 200/400", up, down)
	}
	if state.TrafficBatchSeq != 3 {
		t.Fatalf("batch seq = %d, want 3", state.TrafficBatchSeq)
	}
	var seq2 [][]agentclient.TrafficRecord
	for _, batch := range sent {
		if batch.BatchSeq == 2 {
			seq2 = append(seq2, batch.Records)
		}
	}
	if len(seq2) < 2 {
		t.Fatalf("expected repeated seq 2 attempts, got %d", len(seq2))
	}
	if seq2[0][0].U != seq2[1][0].U || seq2[0][0].D != seq2[1][0].D {
		t.Fatalf("duplicate batch_seq must replay an identical payload, got %+v then %+v", seq2[0], seq2[1])
	}
}

func TestTelemetryDeviceSnapshot(t *testing.T) {
	var mu sync.Mutex
	var got agentclient.DeviceBatch
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/devices" {
			http.NotFound(w, r)
			return
		}
		var batch agentclient.DeviceBatch
		_ = json.NewDecoder(r.Body).Decode(&batch)
		mu.Lock()
		got = batch
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(agentclient.DeviceAck{Accepted: true, BatchSeq: batch.BatchSeq, Devices: int64(len(batch.Devices))})
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{}
	kernel.setSnapshot(kernelsingbox.Snapshot{
		Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{},
		Devices: []kernelsingbox.Device{{
			UserID: 1, NodeID: 7, IPs: []string{"203.0.113.9"}, Online: 2,
		}},
	})
	loop := newTestLoop(t, client, kernel, &agentstate.State{})
	loop.telemetryPass(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(got.Devices) != 1 {
		t.Fatalf("expected 1 device report, got %+v", got.Devices)
	}
	if got.Devices[0].UserID != 1 || got.Devices[0].NodeID != 7 || got.Devices[0].Online != 2 {
		t.Fatalf("unexpected device payload %+v", got.Devices[0])
	}
	if len(got.Devices[0].IPs) != 1 || got.Devices[0].IPs[0] != "203.0.113.9" {
		t.Fatalf("unexpected device ips %+v", got.Devices[0].IPs)
	}
}

func TestCollectDeltasIgnoresCounterReset(t *testing.T) {
	loop := newTestLoop(t, nil, &stubKernel{}, &agentstate.State{})
	pair := kernelsingbox.Pair{UserID: 1, NodeID: 7}
	loop.collectDeltas(map[kernelsingbox.Pair]kernelsingbox.Traffic{pair: {Upload: 100, Download: 100}})
	loop.collectDeltas(map[kernelsingbox.Pair]kernelsingbox.Traffic{pair: {Upload: 5, Download: 5}})
	loop.mu.Lock()
	defer loop.mu.Unlock()
	pending := loop.pending[pair]
	if pending.Upload != 100 || pending.Download != 100 {
		t.Fatalf("counter reset must not add negative traffic, got %+v", pending)
	}
}

func TestTelemetryPermanentRejectionRequeuesTraffic(t *testing.T) {
	var mu sync.Mutex
	var applied []agentclient.TrafficRecord
	rejectSeq := int64(1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/agent/traffic":
			var batch agentclient.TrafficBatch
			_ = json.NewDecoder(r.Body).Decode(&batch)
			mu.Lock()
			reject := batch.BatchSeq == rejectSeq
			if !reject {
				applied = append(applied, batch.Records...)
			}
			mu.Unlock()
			if reject {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"error":{"code":"validation","message":"recorded_at is outside the acceptance window"}}`))
				return
			}
			_ = json.NewEncoder(w).Encode(agentclient.TrafficAck{Accepted: true, BatchSeq: batch.BatchSeq, Records: int64(len(batch.Records))})
		case "/api/agent/devices":
			var batch agentclient.DeviceBatch
			_ = json.NewDecoder(r.Body).Decode(&batch)
			_ = json.NewEncoder(w).Encode(agentclient.DeviceAck{Accepted: true, BatchSeq: batch.BatchSeq, Devices: int64(len(batch.Devices))})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{}
	state := &agentstate.State{}
	loop := newTestLoop(t, client, kernel, state)
	pair := kernelsingbox.Pair{UserID: 1, NodeID: 7}
	ctx := context.Background()

	kernel.setSnapshot(kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{pair: {Upload: 100, Download: 200}}})
	loop.telemetryPass(ctx)

	kernel.setSnapshot(kernelsingbox.Snapshot{Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{pair: {Upload: 150, Download: 260}}})
	loop.telemetryPass(ctx)

	mu.Lock()
	defer mu.Unlock()
	var up, down int64
	for _, rec := range applied {
		up += rec.U
		down += rec.D
	}
	if up != 150 || down != 260 {
		t.Fatalf("permanently rejected deltas must be requeued and applied, got up=%d down=%d", up, down)
	}
	if state.TrafficBatchSeq != 2 {
		t.Fatalf("requeued batch must use a fresh sequence, got %d", state.TrafficBatchSeq)
	}
}

func TestTelemetryPermanentRejectionRebuildsDeviceSnapshot(t *testing.T) {
	var mu sync.Mutex
	var accepted []agentclient.DeviceReport
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/agent/devices" {
			http.NotFound(w, r)
			return
		}
		var batch agentclient.DeviceBatch
		_ = json.NewDecoder(r.Body).Decode(&batch)
		if batch.BatchSeq == 1 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":{"code":"validation","message":"recorded_at is outside the acceptance window"}}`))
			return
		}
		mu.Lock()
		accepted = append(accepted, batch.Devices...)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(agentclient.DeviceAck{Accepted: true, BatchSeq: batch.BatchSeq, Devices: int64(len(batch.Devices))})
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{}
	state := &agentstate.State{}
	loop := newTestLoop(t, client, kernel, state)
	ctx := context.Background()

	kernel.setSnapshot(kernelsingbox.Snapshot{
		Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{},
		Devices: []kernelsingbox.Device{{UserID: 1, NodeID: 7, IPs: []string{"1.1.1.1"}, Online: 1}},
	})
	loop.telemetryPass(ctx)

	kernel.setSnapshot(kernelsingbox.Snapshot{
		Traffic: map[kernelsingbox.Pair]kernelsingbox.Traffic{},
		Devices: []kernelsingbox.Device{{UserID: 1, NodeID: 7, IPs: []string{"2.2.2.2"}, Online: 1}},
	})
	loop.telemetryPass(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(accepted) != 1 || accepted[0].IPs[0] != "2.2.2.2" {
		t.Fatalf("permanently rejected snapshot must be rebuilt from the latest state, got %+v", accepted)
	}
	if state.DeviceBatchSeq != 2 {
		t.Fatalf("rebuilt snapshot must use a fresh sequence, got %d", state.DeviceBatchSeq)
	}
}

func TestTelemetryVisitReplayKeepsNewVisits(t *testing.T) {
	var mu sync.Mutex
	accepted := map[string]int{}
	remainingFailures := 4
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/agent/visits" {
			http.NotFound(w, r)
			return
		}
		var batch agentclient.VisitBatch
		_ = json.NewDecoder(r.Body).Decode(&batch)
		mu.Lock()
		fail := remainingFailures > 0
		if fail {
			remainingFailures--
		}
		if fail {
			mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"internal","message":"boom"}}`))
			return
		}
		for _, rec := range batch.Records {
			accepted[rec.DestHost]++
		}
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(agentclient.VisitAck{Accepted: true, BatchSeq: batch.BatchSeq, Records: int64(len(batch.Records))})
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{}
	state := &agentstate.State{}
	loop := NewLoop(LoopOptions{
		Config:    &config.Agent{Collection: config.Collection{Traffic: true, Visits: true}},
		Client:    client,
		State:     state,
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Kernel:    kernel,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx := context.Background()

	kernel.setVisits([]kernelsingbox.Visit{{
		UserID: 1, NodeID: 7, DestHost: "first.example.com", DestPort: 443, Network: "tcp", At: time.Now().UTC(),
	}})
	loop.telemetryPass(ctx)

	kernel.setVisits([]kernelsingbox.Visit{{
		UserID: 1, NodeID: 7, DestHost: "second.example.com", DestPort: 443, Network: "tcp", At: time.Now().UTC(),
	}})
	loop.telemetryPass(ctx)

	loop.telemetryPass(ctx)

	mu.Lock()
	defer mu.Unlock()
	if accepted["first.example.com"] != 1 || accepted["second.example.com"] != 1 {
		t.Fatalf("retryable replay must not drop newly drained visits, got %v", accepted)
	}
	if state.VisitBatchSeq != 2 {
		t.Fatalf("expected VisitBatchSeq 2, got %d", state.VisitBatchSeq)
	}
}

func TestBuildVisitBatchesSortsAndChunks(t *testing.T) {
	at := time.Date(2026, 9, 22, 10, 0, 0, 0, time.UTC)
	visits := make([]kernelsingbox.Visit, 0, maxBatchRecords+1)
	for i := 0; i < maxBatchRecords; i++ {
		visits = append(visits, kernelsingbox.Visit{
			UserID: 2, NodeID: 7, DestHost: "b.example.com", DestPort: 443,
			Network: "tcp", ClientIP: "1.1.1.1", At: at,
		})
	}
	visits = append(visits, kernelsingbox.Visit{
		UserID: 1, NodeID: 3, DestHost: "a.example.com", DestPort: 80,
		Network: "tcp", ClientIP: "1.1.1.1", At: at,
	})

	batches := buildVisitBatches(visits, 5)
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	if batches[0].BatchSeq != 6 || batches[1].BatchSeq != 7 {
		t.Fatalf("unexpected batch seqs %d %d", batches[0].BatchSeq, batches[1].BatchSeq)
	}
	if len(batches[0].Records) != maxBatchRecords || len(batches[1].Records) != 1 {
		t.Fatalf("unexpected chunk sizes %d %d", len(batches[0].Records), len(batches[1].Records))
	}
	if batches[0].Records[0].UserID != 1 || batches[0].Records[0].DestHost != "a.example.com" {
		t.Fatalf("records must be sorted by user/node/host, got %+v", batches[0].Records[0])
	}
	if batches[0].Records[0].RecordedAt != at.Format(time.RFC3339) {
		t.Fatalf("unexpected recorded_at %q", batches[0].Records[0].RecordedAt)
	}
	if got := buildVisitBatches(nil, 0); got != nil {
		t.Fatalf("empty visits must not produce a batch, got %+v", got)
	}
}

func TestTelemetryReportsVisits(t *testing.T) {
	var mu sync.Mutex
	var accepted []agentclient.VisitRecord
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/agent/visits" {
			http.NotFound(w, r)
			return
		}
		var batch agentclient.VisitBatch
		_ = json.NewDecoder(r.Body).Decode(&batch)
		mu.Lock()
		accepted = append(accepted, batch.Records...)
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(agentclient.VisitAck{Accepted: true, BatchSeq: batch.BatchSeq, Records: int64(len(batch.Records))})
	}))
	defer srv.Close()

	client, err := agentclient.New(srv.URL)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	kernel := &stubKernel{}
	state := &agentstate.State{}
	loop := NewLoop(LoopOptions{
		Config:    &config.Agent{Collection: config.Collection{Traffic: true, Visits: true}},
		Client:    client,
		State:     state,
		StatePath: filepath.Join(t.TempDir(), "state.json"),
		Kernel:    kernel,
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	ctx := context.Background()

	kernel.setVisits([]kernelsingbox.Visit{{
		UserID: 1, NodeID: 7, DestHost: "example.com", DestPort: 443,
		Network: "tcp", ClientIP: "1.1.1.1", At: time.Now().UTC(),
	}})
	loop.telemetryPass(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(accepted) != 1 || accepted[0].DestHost != "example.com" {
		t.Fatalf("expected 1 visit to be reported, got %+v", accepted)
	}
	if state.VisitBatchSeq != 1 {
		t.Fatalf("expected VisitBatchSeq 1, got %d", state.VisitBatchSeq)
	}
	if kernel.DrainVisits() != nil {
		t.Fatal("drained visits must be cleared")
	}
}
