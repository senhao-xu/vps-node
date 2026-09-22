package repo_test

import (
	"context"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func TestIngestVisitBatchIdempotentAndAggregates(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	serverID := mustCreateServer(t, r, "s1")
	nodeID := mustCreateNode(t, r, serverID, "n1", 443)
	userID := mustCreateUser(t, r, "uuid-visit")
	agentID, err := r.CreateAgent(ctx, serverID, "agent-hash", "")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	first := time.Date(2026, 9, 21, 23, 59, 0, 0, time.UTC)
	second := time.Date(2026, 9, 22, 0, 1, 0, 0, time.UTC)
	records := []repo.NewVisitRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, DestHost: "a.example.com", DestPort: 443, Network: "tcp", ClientIP: "1.1.1.1", CreatedAt: first},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, DestHost: "a.example.com", DestPort: 443, Network: "tcp", ClientIP: "1.1.1.1", CreatedAt: first},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, DestHost: "b.example.com", DestPort: 80, Network: "tcp", ClientIP: "2.2.2.2", CreatedAt: second},
	}

	count, dup, err := r.IngestVisitBatch(ctx, agentID, 1, serverID, records)
	if err != nil || dup || count != 3 {
		t.Fatalf("ingest count=%d dup=%v err=%v", count, dup, err)
	}

	count, dup, err = r.IngestVisitBatch(ctx, agentID, 1, serverID, records)
	if err != nil || !dup || count != 3 {
		t.Fatalf("duplicate count=%d dup=%v err=%v", count, dup, err)
	}

	items, total, err := r.ListVisits(ctx, repo.VisitFilter{})
	if err != nil || total != 3 || len(items) != 3 {
		t.Fatalf("list total=%d len=%d err=%v", total, len(items), err)
	}
	if items[0].Username != "user-uuid-visit" || items[0].NodeName != "n1" || items[0].ServerName != "s1" {
		t.Fatalf("visit must join display names, got %+v", items[0])
	}

	filtered, filteredTotal, err := r.ListVisits(ctx, repo.VisitFilter{Host: "a."})
	if err != nil || filteredTotal != 2 || len(filtered) != 2 {
		t.Fatalf("host filter total=%d len=%d err=%v", filteredTotal, len(filtered), err)
	}

	paged, pagedTotal, err := r.ListVisits(ctx, repo.VisitFilter{Page: 1, PageSize: 1})
	if err != nil || pagedTotal != 3 || len(paged) != 1 {
		t.Fatalf("pagination total=%d len=%d err=%v", pagedTotal, len(paged), err)
	}

	all, err := r.TopVisitHosts(ctx, repo.VisitFilter{}, 0, 10)
	if err != nil || len(all) != 2 {
		t.Fatalf("top hosts len=%d err=%v", len(all), err)
	}
	if all[0].DestHost != "a.example.com" || all[0].Hits != 2 {
		t.Fatalf("expected a.example.com hits 2, got %+v", all[0])
	}
	if all[1].DestHost != "b.example.com" || all[1].Hits != 1 {
		t.Fatalf("expected b.example.com hits 1, got %+v", all[1])
	}

	day := time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC).Unix()
	sinceNextDay, err := r.TopVisitHosts(ctx, repo.VisitFilter{}, day, 10)
	if err != nil || len(sinceNextDay) != 1 || sinceNextDay[0].DestHost != "b.example.com" {
		t.Fatalf("day cutoff must exclude prior days, got %+v err=%v", sinceNextDay, err)
	}

	foreign, err := r.TopVisitHosts(ctx, repo.VisitFilter{UserID: userID + 1}, 0, 10)
	if err != nil || len(foreign) != 0 {
		t.Fatalf("user filter must exclude other users, got %+v err=%v", foreign, err)
	}
}
