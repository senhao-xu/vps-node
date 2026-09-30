package repo_test

import (
	"context"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func TestTrafficDepletionBumpsEveryAssignedServerOnce(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	now := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
	r.Now = func() time.Time { return now }
	serverA := mustCreateServer(t, r, "a")
	serverB := mustCreateServer(t, r, "b")
	unrelated := mustCreateServer(t, r, "unrelated")
	nodeA := mustCreateNode(t, r, serverA, "a", 443)
	nodeB := mustCreateNode(t, r, serverB, "b", 443)
	userID := mustCreateUser(t, r, "limited")
	if _, err := r.SetUserNodesAndBump(ctx, userID, []int64{nodeA, nodeB}); err != nil {
		t.Fatal(err)
	}
	unlimited, err := r.CreateUser(ctx, repo.NewUser{UUID: "unlimited", Username: "unlimited", TokenHash: "unlimited"})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := r.CreateUser(ctx, repo.NewUser{UUID: "disabled", Username: "disabled", TokenHash: "disabled", Status: repo.UserStatusDisabled, TransferEnable: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{unlimited, disabled} {
		if _, err := r.SetUserNodesAndBump(ctx, id, []int64{nodeB}); err != nil {
			t.Fatal(err)
		}
	}
	agent, err := r.CreateAgent(ctx, serverA, "key", "")
	if err != nil {
		t.Fatal(err)
	}
	revisions := map[int64]int64{}
	for _, id := range []int64{serverA, serverB, unrelated} {
		revisions[id], err = r.GetServerRevision(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	assertRevisions := func(bumped bool) {
		t.Helper()
		for id, original := range revisions {
			want := original
			if bumped && id != unrelated {
				want++
			}
			actual, err := r.GetServerRevision(ctx, id)
			if err != nil || actual != want {
				t.Fatalf("server %d revision=%d want=%d: %v", id, actual, want, err)
			}
		}
	}
	batch := func(seq, used int64) {
		t.Helper()
		_, dup, err := r.IngestTrafficBatch(ctx, agent, seq, []repo.NewTrafficRecord{{UserID: userID, NodeID: nodeA, ServerID: serverA, U: used, CreatedAt: now}})
		if err != nil || dup {
			t.Fatalf("ingest seq %d duplicate=%v: %v", seq, dup, err)
		}
	}
	batch(1, 99)
	assertRevisions(false)
	batch(2, 1)
	assertRevisions(true)
	if _, dup, err := r.IngestTrafficBatch(ctx, agent, 2, nil); err != nil || !dup {
		t.Fatalf("duplicate=%v: %v", dup, err)
	}
	assertRevisions(true)
	batch(3, 20)
	assertRevisions(true)
	if _, _, err := r.IngestTrafficBatch(ctx, agent, 4, []repo.NewTrafficRecord{
		{UserID: unlimited, NodeID: nodeA, ServerID: serverA, U: 100, CreatedAt: now},
		{UserID: disabled, NodeID: nodeA, ServerID: serverA, U: 1, CreatedAt: now},
	}); err != nil {
		t.Fatal(err)
	}
	assertRevisions(true)
	eligible, err := r.ListEligibleUsersByServer(ctx, serverA, now)
	if err != nil || len(eligible) != 0 {
		t.Fatalf("depleted eligibility=%v: %v", eligible, err)
	}
}

func TestTrafficBatchRollsBackUsageAndRevisionTogether(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	server := mustCreateServer(t, r, "a")
	node := mustCreateNode(t, r, server, "node", 443)
	user := mustCreateUser(t, r, "user")
	if _, err := r.SetUserNodesAndBump(ctx, user, []int64{node}); err != nil {
		t.Fatal(err)
	}
	agent, err := r.CreateAgent(ctx, server, "key", "")
	if err != nil {
		t.Fatal(err)
	}
	original, err := r.GetServerRevision(ctx, server)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.DB.ExecContext(ctx, `CREATE TRIGGER reject_batch BEFORE INSERT ON traffic_batches BEGIN SELECT RAISE(ABORT, 'reject batch'); END`); err != nil {
		t.Fatal(err)
	}
	records := []repo.NewTrafficRecord{{UserID: user, NodeID: node, ServerID: server, U: 100, CreatedAt: time.Now()}}
	if _, _, err := r.IngestTrafficBatch(ctx, agent, 1, records); err == nil {
		t.Fatal("expected batch rejection")
	}
	u, err := r.GetUser(ctx, user)
	if err != nil || u.UsedBytes() != 0 {
		t.Fatalf("user totals survived rollback: %+v %v", u, err)
	}
	revision, err := r.GetServerRevision(ctx, server)
	if err != nil || revision != original {
		t.Fatalf("revision survived rollback: %d %v", revision, err)
	}
	for _, table := range []string{"traffic_records", "server_traffic_daily", "traffic_batches"} {
		var count int
		if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s survived rollback: %d %v", table, count, err)
		}
	}
	s, err := r.GetServer(ctx, server)
	if err != nil || s.TrafficUsedBytes != 0 {
		t.Fatalf("server total survived rollback: %d %v", s.TrafficUsedBytes, err)
	}
	if _, err := r.DB.ExecContext(ctx, `DROP TRIGGER reject_batch`); err != nil {
		t.Fatal(err)
	}
	if _, dup, err := r.IngestTrafficBatch(ctx, agent, 1, records); err != nil || dup {
		t.Fatalf("retry duplicate=%v: %v", dup, err)
	}
	revision, err = r.GetServerRevision(ctx, server)
	if err != nil || revision != original+1 {
		t.Fatalf("retry revision=%d: %v", revision, err)
	}
}

func TestNaturalExpiryReconciliationPersistsAcrossRepoRestart(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	start := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC)
	expiry := start.Add(time.Minute)
	a := mustCreateServer(t, r, "a")
	b := mustCreateServer(t, r, "b")
	nodeA := mustCreateNode(t, r, a, "a", 443)
	nodeB := mustCreateNode(t, r, b, "b", 443)
	user := mustCreateUser(t, r, "user")
	if err := r.SetUserExpiry(ctx, user, nil, &expiry); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SetUserNodesAndBump(ctx, user, []int64{nodeA, nodeB}); err != nil {
		t.Fatal(err)
	}
	initial := map[int64]int64{}
	for _, id := range []int64{a, b} {
		revision, err := r.ReconcileServerExpiry(ctx, id, start)
		if err != nil {
			t.Fatal(err)
		}
		initial[id] = revision
	}
	for _, id := range []int64{a, b} {
		revision, err := r.ReconcileServerExpiry(ctx, id, expiry)
		if err != nil || revision != initial[id]+1 {
			t.Fatalf("expiry %d revision=%d: %v", id, revision, err)
		}
		restarted := repo.New(r.DB)
		revision, err = restarted.ReconcileServerExpiry(ctx, id, expiry.Add(time.Hour))
		if err != nil || revision != initial[id]+1 {
			t.Fatalf("repeated expiry %d revision=%d: %v", id, revision, err)
		}
		revision, err = restarted.ReconcileServerExpiry(ctx, id, start)
		if err != nil || revision != initial[id]+1 {
			t.Fatalf("clock rollback %d revision=%d: %v", id, revision, err)
		}
	}
	u, err := r.GetUser(ctx, user)
	if err != nil || u.Status != repo.UserStatusActive {
		t.Fatalf("natural expiry must leave stored status active: %+v %v", u, err)
	}
	for _, id := range []int64{a, b} {
		eligible, err := r.ListEligibleUsersByServer(ctx, id, expiry)
		if err != nil || len(eligible) != 0 {
			t.Fatalf("expired eligibility=%v: %v", eligible, err)
		}
	}
}

func TestServerMonthlyUsageSurvivesRetentionAndCap(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	server := mustCreateServer(t, r, "server")
	node, err := r.CreateNode(ctx, repo.NewNode{ServerID: server, Name: "node", Protocol: repo.ProtocolHTTP, Port: 8080, Rate: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	user := mustCreateUser(t, r, "user")
	agent, err := r.CreateAgent(ctx, server, "key", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.March, 20, 12, 0, 0, 0, time.FixedZone("offset", 8*3600))
	boundary := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	records := []repo.NewTrafficRecord{
		{UserID: user, NodeID: node, ServerID: server, U: 100, D: 100, CreatedAt: boundary.Add(-time.Second)},
		{UserID: user, NodeID: node, ServerID: server, U: 10, D: 20, CreatedAt: boundary},
		{UserID: user, NodeID: node, ServerID: server, U: 6, D: 14, CreatedAt: boundary.AddDate(0, 0, 3)},
		{UserID: user, NodeID: node, ServerID: server, U: 2, D: 4, CreatedAt: boundary.AddDate(0, 0, 4)},
	}
	if _, _, err := r.IngestTrafficBatch(ctx, agent, 1, records); err != nil {
		t.Fatal(err)
	}
	if _, dup, err := r.IngestTrafficBatch(ctx, agent, 1, records); err != nil || !dup {
		t.Fatalf("duplicate=%v: %v", dup, err)
	}
	assertUsage := func() {
		t.Helper()
		s, err := r.GetServer(ctx, server)
		if err != nil {
			t.Fatal(err)
		}
		if s.TrafficUsedBytes != 384 {
			t.Fatalf("cumulative bytes=%d", s.TrafficUsedBytes)
		}
		s.TrafficResetDay = 15
		for _, mode := range []string{"sum", "max"} {
			s.TrafficAccounting = mode
			up, down, used, err := r.ServerMonthlyUsage(ctx, s, now)
			want := int64(84)
			if mode == "max" {
				want = 57
			}
			if err != nil || up != 27 || down != 57 || used != want {
				t.Fatalf("monthly %s = %d/%d/%d want=27/57/%d: %v", mode, up, down, used, want, err)
			}
		}
	}
	assertUsage()
	if count, err := r.DeleteTrafficRecordsBatch(ctx, boundary.AddDate(0, 0, 4), 500); err != nil || count != 3 {
		t.Fatalf("retention deleted=%d: %v", count, err)
	}
	assertUsage()
	if count, err := r.DeleteTrafficRecordsBeyondCap(ctx, 0, 500); err != nil || count != 1 {
		t.Fatalf("cap deleted=%d: %v", count, err)
	}
	assertUsage()
	if err := r.InsertTrafficRecords(ctx, []repo.NewTrafficRecord{{UserID: user, NodeID: node, ServerID: server, U: 1, D: 2, CreatedAt: boundary.AddDate(0, 1, 0)}}); err != nil {
		t.Fatal(err)
	}
	s, err := r.GetServer(ctx, server)
	if err != nil {
		t.Fatal(err)
	}
	s.TrafficResetDay = 15
	s.TrafficAccounting = "sum"
	up, down, used, err := r.ServerMonthlyUsage(ctx, s, boundary.AddDate(0, 1, 1))
	if err != nil || up != 1 || down != 2 || used != 3 {
		t.Fatalf("next cycle=%d/%d/%d: %v", up, down, used, err)
	}
}

func TestDeleteServerRefreshesOnlineCountsAcrossServers(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	a := mustCreateServer(t, r, "a")
	b := mustCreateServer(t, r, "b")
	nodeA := mustCreateNode(t, r, a, "a", 443)
	nodeB := mustCreateNode(t, r, b, "b", 443)
	onlyA := mustCreateUser(t, r, "only-a")
	both := mustCreateUser(t, r, "both")
	seen := time.Now().Unix()
	agentA, err := r.CreateAgent(ctx, a, "key-a", "")
	if err != nil {
		t.Fatal(err)
	}
	agentB, err := r.CreateAgent(ctx, b, "key-b", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.IngestDeviceBatch(ctx, agentA, 1, a, []repo.NewOnlineDevice{
		{UserID: onlyA, NodeID: nodeA, IP: "one", Online: 1},
		{UserID: both, NodeID: nodeA, IP: "shared", Online: 1},
		{UserID: both, NodeID: nodeA, IP: "only-a", Online: 1},
	}, map[int64]int{onlyA: 1, both: 2}, seen); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.IngestDeviceBatch(ctx, agentB, 1, b, []repo.NewOnlineDevice{
		{UserID: both, NodeID: nodeB, IP: "shared", Online: 1},
	}, map[int64]int{both: 1}, seen); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]int64{onlyA: 1, both: 2} {
		u, err := r.GetUser(ctx, id)
		if err != nil || u.OnlineCount != want {
			t.Fatalf("before deletion user %d online=%d want=%d: %v", id, u.OnlineCount, want, err)
		}
	}
	if _, err := r.DB.ExecContext(ctx, `CREATE TRIGGER reject_online_refresh BEFORE UPDATE OF online_count ON users BEGIN SELECT RAISE(ABORT, 'reject refresh'); END`); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteServerCascade(ctx, a); err == nil {
		t.Fatal("expected online count failure to roll back server deletion")
	}
	if _, err := r.GetServer(ctx, a); err != nil {
		t.Fatalf("server deletion survived rollback: %v", err)
	}
	devices, err := r.ListDevicesByUser(ctx, onlyA)
	if err != nil || len(devices) != 1 {
		t.Fatalf("devices deleted despite rollback: %v %v", devices, err)
	}
	if _, err := r.DB.ExecContext(ctx, `DROP TRIGGER reject_online_refresh`); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteServerCascade(ctx, a); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]int64{onlyA: 0, both: 1} {
		u, err := r.GetUser(ctx, id)
		if err != nil || u.OnlineCount != want {
			t.Fatalf("user %d online=%d want=%d: %v", id, u.OnlineCount, want, err)
		}
	}
}

func TestRecreatedIdentitiesCannotReadRetainedHistory(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	oldServer := mustCreateServer(t, r, "old")
	oldNode := mustCreateNode(t, r, oldServer, "old", 443)
	oldUser := mustCreateUser(t, r, "old")
	agent, err := r.CreateAgent(ctx, oldServer, "key", "")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if err := r.InsertTrafficRecords(ctx, []repo.NewTrafficRecord{{UserID: oldUser, NodeID: oldNode, ServerID: oldServer, U: 10, D: 20, CreatedAt: at}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.IngestVisitBatch(ctx, agent, 1, oldServer, []repo.NewVisitRecord{{UserID: oldUser, NodeID: oldNode, DestHost: "private.example", ClientIP: "192.0.2.10", CreatedAt: at}}); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteUserAndBump(ctx, oldUser); err != nil {
		t.Fatal(err)
	}
	user := mustCreateUser(t, r, "new")
	if user <= oldUser {
		t.Fatalf("reused user id=%d", user)
	}
	if err := r.DeleteNode(ctx, oldNode); err != nil {
		t.Fatal(err)
	}
	node := mustCreateNode(t, r, oldServer, "new", 443)
	if node <= oldNode {
		t.Fatalf("reused node id=%d", node)
	}
	if err := r.DeleteServerCascade(ctx, oldServer); err != nil {
		t.Fatal(err)
	}
	server := mustCreateServer(t, r, "new")
	if server <= oldServer {
		t.Fatalf("reused server id=%d", server)
	}
	node2, err := r.CreateNodeAndBump(ctx, repo.NewNode{ServerID: server, Name: "new-2", Protocol: repo.ProtocolHTTP, Port: 8080})
	if err != nil || node2 <= node {
		t.Fatalf("mutation reused node=%d: %v", node2, err)
	}
	user2, err := r.CreateUserWithNodes(ctx, repo.NewUser{UUID: "new-2", Username: "new-2", TokenHash: "new-2"}, []int64{node2})
	if err != nil || user2 <= user {
		t.Fatalf("mutation reused user=%d: %v", user2, err)
	}
	if err := r.DeleteServerCascade(ctx, server); err != nil {
		t.Fatal(err)
	}
	server2, err := r.CreateServerWithAgentKey(ctx, "new-2", repo.ServerStatusActive, "new-key", []byte{1}, repo.ServerInventory{})
	if err != nil || server2 <= server {
		t.Fatalf("mutation reused server=%d: %v", server2, err)
	}
	for _, f := range []repo.TrafficFilter{{UserID: user}, {UserID: user2}, {NodeID: node}, {NodeID: node2}, {ServerID: server}, {ServerID: server2}} {
		up, down, err := r.SumTraffic(ctx, f)
		if err != nil || up != 0 || down != 0 {
			t.Fatalf("new identity inherited traffic %+v=%d/%d: %v", f, up, down, err)
		}
	}
	for _, f := range []repo.VisitFilter{{UserID: user}, {UserID: user2}, {NodeID: node}, {NodeID: node2}, {ServerID: server}, {ServerID: server2}} {
		visits, total, err := r.ListVisits(ctx, f)
		if err != nil || total != 0 || len(visits) != 0 {
			t.Fatalf("new identity inherited visits %+v=%v: %v", f, visits, err)
		}
		hosts, err := r.TopVisitHosts(ctx, f, 0, 10)
		if err != nil || len(hosts) != 0 {
			t.Fatalf("new identity inherited host aggregates %+v=%v: %v", f, hosts, err)
		}
	}
	s, err := r.GetServer(ctx, server2)
	if err != nil {
		t.Fatal(err)
	}
	up, down, used, err := r.ServerMonthlyUsage(ctx, s, at)
	if err != nil || up != 0 || down != 0 || used != 0 || s.TrafficUsedBytes != 0 {
		t.Fatalf("new server inherited usage %d/%d/%d: %v", up, down, used, err)
	}
	for _, table := range []string{"traffic_records", "visit_records", "visit_daily_domains", "server_traffic_daily"} {
		var count int
		if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("retained history %s count=%d: %v", table, count, err)
		}
	}
}
