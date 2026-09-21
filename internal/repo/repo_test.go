package repo_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"vps-node/internal/db"
	"vps-node/internal/repo"
)

func newTestRepo(t *testing.T) *repo.Repo {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return repo.New(d.DB)
}

func mustCreateUser(t *testing.T, r *repo.Repo, uuid string) int64 {
	t.Helper()
	id, err := r.CreateUser(context.Background(), repo.NewUser{UUID: uuid, Username: "user-" + uuid, TokenHash: "hash-" + uuid, Status: repo.UserStatusActive, QuotaBytes: 100})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func mustCreateServer(t *testing.T, r *repo.Repo, name string) int64 {
	t.Helper()
	id, err := r.CreateServer(context.Background(), name, "1.2.3.4", repo.ServerStatusActive)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return id
}

func mustCreateNode(t *testing.T, r *repo.Repo, serverID int64, name string, port int) int64 {
	t.Helper()
	id, err := r.CreateNode(context.Background(), repo.NewNode{ServerID: serverID, Name: name, Protocol: repo.ProtocolVLESS, Port: port})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	return id
}

func TestUserCRUD(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	id := mustCreateUser(t, r, "uuid-1")

	u, err := r.GetUser(ctx, id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.UUID != "uuid-1" || u.Status != repo.UserStatusActive || u.UsedBytes != 0 {
		t.Fatalf("unexpected user: %+v", u)
	}

	byUUID, err := r.GetUserByUUID(ctx, "uuid-1")
	if err != nil {
		t.Fatalf("get by uuid: %v", err)
	}
	if byUUID.ID != id {
		t.Fatalf("uuid lookup mismatch: %d != %d", byUUID.ID, id)
	}

	if _, err := r.CreateUser(ctx, repo.NewUser{UUID: "uuid-1", Username: "user-x", TokenHash: "other"}); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("expected conflict on duplicate uuid, got %v", err)
	}
	if _, err := r.CreateUser(ctx, repo.NewUser{UUID: "uuid-2", Username: "user-y", TokenHash: "hash-uuid-1"}); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("expected conflict on duplicate token hash, got %v", err)
	}
	if _, err := r.CreateUser(ctx, repo.NewUser{UUID: "uuid-3", Username: "user-uuid-1", TokenHash: "hash-uuid-3"}); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("expected conflict on duplicate username, got %v", err)
	}

	if err := r.SetUserStatus(ctx, id, repo.UserStatusDisabled); err != nil {
		t.Fatalf("set status: %v", err)
	}
	if err := r.SetUserQuota(ctx, id, 500); err != nil {
		t.Fatalf("set quota: %v", err)
	}
	expiry := time.Unix(1800000000, 0)
	if err := r.SetUserExpiry(ctx, id, nil, &expiry); err != nil {
		t.Fatalf("set expiry: %v", err)
	}

	u, err = r.GetUser(ctx, id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.Status != repo.UserStatusDisabled || u.QuotaBytes != 500 || u.ExpiresAt == nil || !u.ExpiresAt.Equal(expiry) {
		t.Fatalf("unexpected updated user: %+v", u)
	}

	if err := r.AddUserUsedBytes(ctx, id, 10, 20); err != nil {
		t.Fatalf("add used: %v", err)
	}
	u, _ = r.GetUser(ctx, id)
	if u.UsedBytes != 30 {
		t.Fatalf("expected used 30, got %d", u.UsedBytes)
	}

	if err := r.ResetUserTraffic(ctx, id); err != nil {
		t.Fatalf("reset traffic: %v", err)
	}
	u, _ = r.GetUser(ctx, id)
	if u.UsedBytes != 0 {
		t.Fatalf("expected used 0 after reset, got %d", u.UsedBytes)
	}

	if err := r.DeleteUser(ctx, id); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if _, err := r.GetUser(ctx, id); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

func TestListUsersFilters(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	id1 := mustCreateUser(t, r, "uuid-a")
	id2 := mustCreateUser(t, r, "uuid-b")
	_ = id2
	if err := r.SetUserStatus(ctx, id2, repo.UserStatusDisabled); err != nil {
		t.Fatalf("set status: %v", err)
	}

	users, total, err := r.ListUsers(ctx, repo.UserFilter{Status: repo.UserStatusActive})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != id1 {
		t.Fatalf("unexpected list result: total=%d users=%+v", total, users)
	}

	users, total, err = r.ListUsers(ctx, repo.UserFilter{Query: "uuid-b"})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if total != 1 || users[0].UUID != "uuid-b" {
		t.Fatalf("unexpected query result: total=%d users=%+v", total, users)
	}

	users, total, err = r.ListUsers(ctx, repo.UserFilter{Query: "user-uuid-a"})
	if err != nil {
		t.Fatalf("list users by username: %v", err)
	}
	if total != 1 || len(users) != 1 || users[0].ID != id1 {
		t.Fatalf("unexpected username query result: total=%d users=%+v", total, users)
	}

	past := time.Now().Add(-time.Hour)
	if err := r.SetUserExpiry(ctx, id2, nil, &past); err != nil {
		t.Fatalf("set expiry: %v", err)
	}
	_, total, err = r.ListUsers(ctx, repo.UserFilter{Expiry: "expired"})
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 expired user, got %d", total)
	}
}

func TestSetUserNodesIdempotent(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	userID := mustCreateUser(t, r, "uuid-n")
	serverID := mustCreateServer(t, r, "s1")
	node1 := mustCreateNode(t, r, serverID, "n1", 443)
	node2 := mustCreateNode(t, r, serverID, "n2", 8443)
	node3 := mustCreateNode(t, r, serverID, "n3", 8444)

	if err := r.SetUserNodes(ctx, userID, []int64{node1, node2, node3, node1, node2}); err != nil {
		t.Fatalf("set user nodes: %v", err)
	}
	if err := r.SetUserNodes(ctx, userID, []int64{node1, node2, node3}); err != nil {
		t.Fatalf("set user nodes again: %v", err)
	}

	ids, err := r.ListNodeIDsByUser(ctx, userID)
	if err != nil {
		t.Fatalf("list node ids: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 unique nodes, got %v", ids)
	}

	count, err := r.CountNodesByUser(ctx, userID)
	if err != nil || count != 3 {
		t.Fatalf("count nodes by user: %d %v", count, err)
	}

	if err := r.SetUserNodes(ctx, userID, nil); err != nil {
		t.Fatalf("clear user nodes: %v", err)
	}
	ids, _ = r.ListNodeIDsByUser(ctx, userID)
	if len(ids) != 0 {
		t.Fatalf("expected empty after clear, got %v", ids)
	}

	if err := r.AuthorizeUserNode(ctx, userID, node1); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := r.AuthorizeUserNode(ctx, userID, node1); err != nil {
		t.Fatalf("authorize twice must be no-op: %v", err)
	}
	ids, _ = r.ListNodeIDsByUser(ctx, userID)
	if len(ids) != 1 {
		t.Fatalf("expected 1 node after authorize, got %v", ids)
	}

	if _, err := r.CreateUser(ctx, repo.NewUser{UUID: "uuid-cascade", Username: "user-cascade", TokenHash: "hash-cascade"}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	cascadeUser, _ := r.GetUserByUUID(ctx, "uuid-cascade")
	if err := r.AuthorizeUserNode(ctx, cascadeUser.ID, node1); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := r.DeleteUser(ctx, cascadeUser.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	userIDs, err := r.ListUserIDsByNode(ctx, node1)
	if err != nil {
		t.Fatalf("list user ids: %v", err)
	}
	for _, uid := range userIDs {
		if uid == cascadeUser.ID {
			t.Fatal("expected cascade delete of user_nodes")
		}
	}
}

func TestAgentOwnershipAndRotation(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	server1 := mustCreateServer(t, r, "s1")
	server2 := mustCreateServer(t, r, "s2")

	agentID, err := r.CreateAgent(ctx, server1, "agent-hash-1", "1.0.0")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if _, err := r.CreateAgent(ctx, server1, "agent-hash-2", ""); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("expected conflict: one agent per server, got %v", err)
	}
	if _, err := r.CreateAgent(ctx, server2, "agent-hash-1", ""); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("expected conflict: duplicate token hash, got %v", err)
	}

	agent, err := r.GetAgentByTokenHash(ctx, "agent-hash-1")
	if err != nil {
		t.Fatalf("get by token hash: %v", err)
	}
	if agent.ServerID != server1 {
		t.Fatalf("expected server %d, got %d", server1, agent.ServerID)
	}

	byServer, err := r.GetAgentByServerID(ctx, server1)
	if err != nil || byServer.ID != agentID {
		t.Fatalf("get by server: %v %+v", err, byServer)
	}

	seen := time.Unix(1800000000, 0)
	if err := r.UpdateAgentHeartbeat(ctx, agentID, "1.1.0", seen); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	agent, _ = r.GetAgent(ctx, agentID)
	if agent.Version != "1.1.0" || agent.LastSeenAt == nil || !agent.LastSeenAt.Equal(seen) {
		t.Fatalf("unexpected agent after heartbeat: %+v", agent)
	}

	if err := r.RotateAgentTokenHash(ctx, agentID, "agent-hash-new"); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if _, err := r.GetAgentByTokenHash(ctx, "agent-hash-1"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected old token invalid, got %v", err)
	}
	if _, err := r.GetAgentByTokenHash(ctx, "agent-hash-new"); err != nil {
		t.Fatalf("expected new token valid, got %v", err)
	}
}

func TestNodeOwnershipConstraints(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	server1 := mustCreateServer(t, r, "s1")
	server2 := mustCreateServer(t, r, "s2")
	node1 := mustCreateNode(t, r, server1, "n1", 443)

	if _, err := r.CreateNode(ctx, repo.NewNode{ServerID: server1, Name: "other", Protocol: repo.ProtocolVLESS, Port: 443}); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("expected port conflict per server, got %v", err)
	}
	if _, err := r.CreateNode(ctx, repo.NewNode{ServerID: server1, Name: "n1", Protocol: repo.ProtocolVLESS, Port: 8443}); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("expected name conflict per server, got %v", err)
	}

	nodes, err := r.ListNodesByServer(ctx, server1)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("list by server: %v %d", err, len(nodes))
	}

	if err := r.DeleteServer(ctx, server2); err != nil {
		t.Fatalf("delete server: %v", err)
	}

	if _, err := r.GetNode(ctx, node1); err != nil {
		t.Fatalf("node of live server must exist: %v", err)
	}
	if err := r.DeleteServer(ctx, server1); err != nil {
		t.Fatalf("delete server: %v", err)
	}
	if _, err := r.GetNode(ctx, node1); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected node cascade deleted, got %v", err)
	}
}

func TestListNodesPageFilters(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	server1 := mustCreateServer(t, r, "s1")
	server2 := mustCreateServer(t, r, "s2")

	mustNode := func(serverID int64, name, protocol string, port int, status string) int64 {
		t.Helper()
		id, err := r.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: name, Protocol: protocol, Port: port, Status: status})
		if err != nil {
			t.Fatalf("create node: %v", err)
		}
		return id
	}
	mustNode(server1, "hk-ss", repo.ProtocolShadowsocks, 8388, repo.NodeStatusActive)
	mustNode(server1, "hk-vless", repo.ProtocolVLESS, 443, repo.NodeStatusDisabled)
	mustNode(server2, "jp-hy2", repo.ProtocolHysteria2, 8443, repo.NodeStatusActive)

	nodes, total, err := r.ListNodesPage(ctx, repo.NodeFilter{})
	if err != nil || total != 3 || len(nodes) != 3 {
		t.Fatalf("list all: %v total=%d len=%d", err, total, len(nodes))
	}
	namesByServer := map[int64]string{server1: "s1", server2: "s2"}
	for _, n := range nodes {
		if n.ServerName != namesByServer[n.ServerID] {
			t.Fatalf("expected server name %q for node %q, got %q", namesByServer[n.ServerID], n.Name, n.ServerName)
		}
	}

	nodes, total, err = r.ListNodesPage(ctx, repo.NodeFilter{Protocol: repo.ProtocolVLESS})
	if err != nil || total != 1 || len(nodes) != 1 || nodes[0].Name != "hk-vless" {
		t.Fatalf("protocol filter: %v total=%d %+v", err, total, nodes)
	}

	nodes, total, err = r.ListNodesPage(ctx, repo.NodeFilter{Status: repo.NodeStatusActive})
	if err != nil || total != 2 || len(nodes) != 2 {
		t.Fatalf("status filter: %v total=%d len=%d", err, total, len(nodes))
	}

	nodes, total, err = r.ListNodesPage(ctx, repo.NodeFilter{Query: "hk-"})
	if err != nil || total != 2 || len(nodes) != 2 {
		t.Fatalf("query filter: %v total=%d len=%d", err, total, len(nodes))
	}

	nodes, total, err = r.ListNodesPage(ctx, repo.NodeFilter{Query: "%"})
	if err != nil || total != 0 || len(nodes) != 0 {
		t.Fatalf("query wildcard must be escaped: %v total=%d len=%d", err, total, len(nodes))
	}

	nodes, total, err = r.ListNodesPage(ctx, repo.NodeFilter{
		ServerID: server1, Protocol: repo.ProtocolShadowsocks, Status: repo.NodeStatusActive, Query: "hk",
		Page: 1, PageSize: 10,
	})
	if err != nil || total != 1 || len(nodes) != 1 || nodes[0].Name != "hk-ss" || nodes[0].ServerName != "s1" {
		t.Fatalf("combined filter: %v total=%d %+v", err, total, nodes)
	}

	nodes, total, err = r.ListNodesPage(ctx, repo.NodeFilter{Page: 2, PageSize: 2})
	if err != nil || total != 3 || len(nodes) != 1 {
		t.Fatalf("pagination: %v total=%d len=%d", err, total, len(nodes))
	}
}

func TestServerRevisionMonotonic(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	serverID := mustCreateServer(t, r, "s1")

	rev, err := r.GetServerRevision(ctx, serverID)
	if err != nil || rev != 0 {
		t.Fatalf("expected initial revision 0, got %d %v", rev, err)
	}

	for want := int64(1); want <= 3; want++ {
		got, err := r.BumpServerRevision(ctx, serverID)
		if err != nil {
			t.Fatalf("bump: %v", err)
		}
		if got != want {
			t.Fatalf("expected revision %d, got %d", want, got)
		}
	}

	rev, _ = r.GetServerRevision(ctx, serverID)
	if rev != 3 {
		t.Fatalf("expected current revision 3, got %d", rev)
	}
}

func TestReplaceServerSessions(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	userID := mustCreateUser(t, r, "uuid-s")
	serverID := mustCreateServer(t, r, "s1")
	nodeID := mustCreateNode(t, r, serverID, "n1", 443)

	now := time.Now().Truncate(time.Second).UTC()
	batch1 := []repo.NewSession{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "1.1.1.1", ConnectedAt: now, LastSeenAt: now},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "2.2.2.2", ConnectedAt: now, LastSeenAt: now},
	}
	if err := r.ReplaceServerSessions(ctx, serverID, batch1); err != nil {
		t.Fatalf("replace sessions: %v", err)
	}

	sessions, err := r.ListSessionsByServer(ctx, serverID, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	batch2 := []repo.NewSession{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "3.3.3.3", ConnectedAt: now, LastSeenAt: now},
	}
	if err := r.ReplaceServerSessions(ctx, serverID, batch2); err != nil {
		t.Fatalf("replace sessions: %v", err)
	}
	sessions, _ = r.ListSessionsByServer(ctx, serverID, now.Add(-time.Minute))
	if len(sessions) != 1 || sessions[0].IP != "3.3.3.3" {
		t.Fatalf("expected snapshot replaced, got %+v", sessions)
	}

	stale, err := r.ListSessionsByServer(ctx, serverID, now.Add(time.Minute))
	if err != nil || len(stale) != 0 {
		t.Fatalf("expected stale sessions filtered, got %d %v", len(stale), err)
	}

	deleted, err := r.DeleteSessionsLastSeenBefore(ctx, now.Add(time.Minute))
	if err != nil || deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d %v", deleted, err)
	}
}

func TestConnectionLogsAndTraffic(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	userID := mustCreateUser(t, r, "uuid-t")
	serverID := mustCreateServer(t, r, "s1")
	nodeID := mustCreateNode(t, r, serverID, "n1", 443)

	connected := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	inserted, err := r.InsertConnectionLogs(ctx, []repo.NewConnectionLog{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "1.1.1.1", Protocol: "vless", UploadBytes: 1, DownloadBytes: 2, ConnectedAt: connected, Status: "closed"},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "2.2.2.2", Protocol: "vless", UploadBytes: 3, DownloadBytes: 4, ConnectedAt: connected, Status: "closed"},
	})
	if err != nil {
		t.Fatalf("insert logs: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("expected 2 inserted, got %d", inserted)
	}

	logs, total, err := r.ListConnectionLogs(ctx, repo.LogFilter{UserID: userID, Page: 1, PageSize: 1})
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if total != 2 || len(logs) != 1 {
		t.Fatalf("unexpected logs page: total=%d len=%d", total, len(logs))
	}

	records := []repo.NewTrafficRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, UploadBytes: 10, DownloadBytes: 20, CreatedAt: connected},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, UploadBytes: 30, DownloadBytes: 40, CreatedAt: connected},
	}
	if err := r.InsertTrafficRecords(ctx, records); err != nil {
		t.Fatalf("insert traffic: %v", err)
	}
	upload, download, err := r.SumTraffic(ctx, repo.TrafficFilter{UserID: userID})
	if err != nil {
		t.Fatalf("sum traffic: %v", err)
	}
	if upload != 40 || download != 60 {
		t.Fatalf("unexpected sums: up=%d down=%d", upload, download)
	}

	if err := r.AddUserUsedBytes(ctx, userID, 40, 60); err != nil {
		t.Fatalf("add used: %v", err)
	}
	u, _ := r.GetUser(ctx, userID)
	if u.UsedBytes != 100 {
		t.Fatalf("expected used 100, got %d", u.UsedBytes)
	}

	cutoff := time.Now().Add(time.Minute)
	logDeleted, err := r.DeleteConnectionLogsBefore(ctx, cutoff)
	if err != nil || logDeleted != 2 {
		t.Fatalf("delete logs: %d %v", logDeleted, err)
	}
	recDeleted, err := r.DeleteTrafficRecordsBefore(ctx, cutoff)
	if err != nil || recDeleted != 2 {
		t.Fatalf("delete records: %d %v", recDeleted, err)
	}
}

func TestBatchIdempotencyTracking(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	serverID := mustCreateServer(t, r, "s1")
	agentID, err := r.CreateAgent(ctx, serverID, "agent-hash", "")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	exists, err := r.TrafficBatchExists(ctx, agentID, 1)
	if err != nil || exists {
		t.Fatalf("expected batch absent: %v %v", exists, err)
	}
	if err := r.RecordTrafficBatch(ctx, agentID, 1); err != nil {
		t.Fatalf("record batch: %v", err)
	}
	exists, _ = r.TrafficBatchExists(ctx, agentID, 1)
	if !exists {
		t.Fatal("expected batch present")
	}

	exists, _ = r.LogBatchExists(ctx, agentID, 7)
	if exists {
		t.Fatal("expected log batch absent")
	}
	if err := r.RecordLogBatch(ctx, agentID, 7); err != nil {
		t.Fatalf("record log batch: %v", err)
	}
	exists, _ = r.LogBatchExists(ctx, agentID, 7)
	if !exists {
		t.Fatal("expected log batch present")
	}
}

func TestTxRollback(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	err := repo.Tx(ctx, r.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO users (uuid, username, token_hash, created_at, updated_at) VALUES ('tx-u', 'user-tx-u', 'tx-h', 1, 1)`); err != nil {
			t.Fatalf("insert in tx: %v", err)
		}
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if _, err := r.GetUserByUUID(ctx, "tx-u"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("expected insert rolled back, got %v", err)
	}
}

func TestSettings(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	value, err := r.GetSettingOr(ctx, "retention.raw_log_days", "7")
	if err != nil || value != "7" {
		t.Fatalf("expected fallback 7, got %q %v", value, err)
	}

	if err := r.SetSetting(ctx, "retention.raw_log_days", "14"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	if err := r.SetSetting(ctx, "retention.raw_log_days", "21"); err != nil {
		t.Fatalf("set setting again: %v", err)
	}

	value, err = r.GetSetting(ctx, "retention.raw_log_days")
	if err != nil || value != "21" {
		t.Fatalf("expected 21, got %q %v", value, err)
	}

	all, err := r.ListSettings(ctx)
	if err != nil || len(all) != 1 || all["retention.raw_log_days"] != "21" {
		t.Fatalf("unexpected settings: %v %v", all, err)
	}
}

func TestAdmins(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	if _, err := r.CreateAdmin(ctx, "root", "hash"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := r.CreateAdmin(ctx, "root", "hash2"); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("expected duplicate username conflict, got %v", err)
	}

	a, err := r.GetAdminByUsername(ctx, "root")
	if err != nil || a.Username != "root" {
		t.Fatalf("get admin: %v %+v", err, a)
	}

	count, err := r.CountAdmins(ctx)
	if err != nil || count != 1 {
		t.Fatalf("count admins: %d %v", count, err)
	}

	if err := r.UpdateAdminPassword(ctx, a.ID, "new-hash"); err != nil {
		t.Fatalf("update password: %v", err)
	}
	a, _ = r.GetAdminByUsername(ctx, "root")
	if a.PasswordHash != "new-hash" {
		t.Fatalf("expected updated hash, got %s", a.PasswordHash)
	}
}
