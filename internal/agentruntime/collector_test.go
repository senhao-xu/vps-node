package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"vps-node/internal/agentclient"
)

type stubSource struct {
	snap *clashConnections
	err  error
}

func (s *stubSource) Connections(context.Context) (*clashConnections, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.snap, nil
}

func testTable() *Table {
	return BuildTable([]agentclient.User{{
		ID: 1001, UUID: "uuid-1001", Nodes: []agentclient.UserNode{{ID: 7, Protocol: "vless", Port: 443}},
	}})
}

func connJSON(t *testing.T, raw string) clashConnections {
	t.Helper()
	var out clashConnections
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return out
}

func TestCollectorDeltasAndClose(t *testing.T) {
	start := time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	src := &stubSource{}
	c := NewCollector(src, testTable())
	ctx := context.Background()
	now := time.Now()

	src.snap = &clashConnections{Connections: []clashConn{{
		ID: "a", Upload: 100, Download: 50, Start: start,
		Metadata: map[string]any{"sourceIP": "1.2.3.4", "inbound": "vless-7", "inboundUser": "u-1001"},
	}}}
	res, err := c.Poll(ctx, now)
	if err != nil {
		t.Fatalf("poll1: %v", err)
	}
	if len(res.Traffic) != 1 || res.Traffic[0].Upload != 100 || res.Traffic[0].Download != 50 {
		t.Fatalf("poll1 traffic: %+v", res.Traffic)
	}
	if len(res.Sessions) != 1 || res.Sessions[0].IP != "1.2.3.4" {
		t.Fatalf("poll1 sessions: %+v", res.Sessions)
	}

	src.snap = &clashConnections{Connections: []clashConn{{
		ID: "a", Upload: 90, Download: 40, Start: start,
		Metadata: map[string]any{"sourceIP": "1.2.3.4", "inbound": "vless-7", "inboundUser": "u-1001"},
	}}}
	res, err = c.Poll(ctx, now)
	if err != nil {
		t.Fatalf("poll2: %v", err)
	}
	if len(res.Traffic) != 0 {
		t.Fatalf("counter regression must not produce negative traffic, got %+v", res.Traffic)
	}

	src.snap = &clashConnections{}
	res, err = c.Poll(ctx, now)
	if err != nil {
		t.Fatalf("poll3: %v", err)
	}
	if len(res.Closed) != 1 || res.Closed[0].Upload != 90 || res.Closed[0].Download != 40 {
		t.Fatalf("poll3 closed: %+v", res.Closed)
	}
	if len(res.Traffic) != 1 || res.Traffic[0].Upload != 90 || res.Traffic[0].Download != 40 {
		t.Fatalf("poll3 final delta traffic: %+v", res.Traffic)
	}
}

func TestCollectorUnattributedExcluded(t *testing.T) {
	start := time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	src := &stubSource{snap: &clashConnections{Connections: []clashConn{
		{ID: "ok", Upload: 10, Download: 10, Start: start,
			Metadata: map[string]any{"sourceIP": "1.2.3.4", "inbound": "vless-7", "inboundUser": "u-1001"}},
		{ID: "no-user", Upload: 99, Download: 99, Start: start,
			Metadata: map[string]any{"sourceIP": "5.6.7.8", "inbound": "vless-7"}},
		{ID: "wrong-user", Upload: 99, Download: 99, Start: start,
			Metadata: map[string]any{"sourceIP": "5.6.7.8", "inbound": "vless-7", "inboundUser": "u-9999"}},
		{ID: "unknown-node", Upload: 99, Download: 99, Start: start,
			Metadata: map[string]any{"sourceIP": "5.6.7.8", "inbound": "vless-13", "inboundUser": "u-1001"}},
		{ID: "no-start", Upload: 99, Download: 99,
			Metadata: map[string]any{"sourceIP": "5.6.7.8", "inbound": "vless-7", "inboundUser": "u-1001"}},
	}}}
	c := NewCollector(src, testTable())

	res, err := c.Poll(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if res.Unattributed != 4 {
		t.Fatalf("expected 4 unattributed, got %d", res.Unattributed)
	}
	if len(res.Traffic) != 1 || res.Traffic[0].Upload != 10 {
		t.Fatalf("only the attributed connection may be reported: %+v", res.Traffic)
	}
}

func TestCollectorReplaceTableDropsRevokedPairs(t *testing.T) {
	start := time.Now().Add(-time.Minute).Format(time.RFC3339Nano)
	src := &stubSource{snap: &clashConnections{Connections: []clashConn{{
		ID: "a", Upload: 10, Download: 10, Start: start,
		Metadata: map[string]any{"sourceIP": "1.2.3.4", "inbound": "vless-7", "inboundUser": "u-1001"},
	}}}}
	c := NewCollector(src, testTable())
	if _, err := c.Poll(context.Background(), time.Now()); err != nil {
		t.Fatalf("poll: %v", err)
	}

	c.ReplaceTable(BuildTable(nil))
	src.snap = &clashConnections{}
	res, err := c.Poll(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("poll after replace: %v", err)
	}
	if len(res.Closed) != 0 || len(res.Traffic) != 0 {
		t.Fatalf("revoked pair data must be dropped, got %+v", res)
	}
}

func TestCollectorSourceError(t *testing.T) {
	src := &stubSource{err: errors.New("clash api down")}
	c := NewCollector(src, testTable())
	if _, err := c.Poll(context.Background(), time.Now()); err == nil {
		t.Fatal("expected source error to propagate")
	}
}

func TestEndpointFromConfig(t *testing.T) {
	raw := json.RawMessage(`{"experimental":{"clash_api":{"external_controller":"127.0.0.1:34567","secret":"s3cret"}}}`)
	base, secret, err := EndpointFromConfig(raw)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	if base != "http://127.0.0.1:34567" || secret != "s3cret" {
		t.Fatalf("unexpected endpoint %q %q", base, secret)
	}
	if _, _, err := EndpointFromConfig(json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error when clash api is absent")
	}
}

func TestNewClashClientRejectsBadURL(t *testing.T) {
	if _, err := NewClashClient("ftp://x", "s"); err == nil {
		t.Fatal("expected error for non-http clash url")
	}
}
