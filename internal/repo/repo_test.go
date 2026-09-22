package repo_test

import (
	"context"
	"database/sql"
	"errors"
	"math"
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
	id, err := r.CreateUser(context.Background(), repo.NewUser{UUID: uuid, Username: "user-" + uuid, TokenHash: "hash-" + uuid, Status: repo.UserStatusActive, TransferEnable: 100})
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
	if u.UUID != "uuid-1" || u.Status != repo.UserStatusActive || u.UsedBytes() != 0 {
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
	if err := r.SetUserTransferEnable(ctx, id, 500); err != nil {
		t.Fatalf("set transfer_enable: %v", err)
	}
	expiry := time.Unix(1800000000, 0)
	if err := r.SetUserExpiry(ctx, id, nil, &expiry); err != nil {
		t.Fatalf("set expiry: %v", err)
	}

	u, err = r.GetUser(ctx, id)
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if u.Status != repo.UserStatusDisabled || u.TransferEnable != 500 || u.ExpiresAt == nil || !u.ExpiresAt.Equal(expiry) {
		t.Fatalf("unexpected updated user: %+v", u)
	}

	if err := r.AddUserUsedBytes(ctx, id, 10, 20); err != nil {
		t.Fatalf("add used: %v", err)
	}
	u, _ = r.GetUser(ctx, id)
	if u.U != 10 || u.D != 20 || u.UsedBytes() != 30 {
		t.Fatalf("expected u=10 d=20, got u=%d d=%d", u.U, u.D)
	}

	if err := r.ResetUserTraffic(ctx, id); err != nil {
		t.Fatalf("reset traffic: %v", err)
	}
	u, _ = r.GetUser(ctx, id)
	if u.U != 0 || u.D != 0 {
		t.Fatalf("expected zero traffic after reset, got u=%d d=%d", u.U, u.D)
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

func TestIngestDeviceBatchPrunesStale(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	userID := mustCreateUser(t, r, "uuid-s")
	serverID := mustCreateServer(t, r, "s1")
	nodeID := mustCreateNode(t, r, serverID, "n1", 443)
	agentID, err := r.CreateAgent(ctx, serverID, "agent-hash", "")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	now := time.Now().Truncate(time.Second).UTC()
	if _, _, err := r.IngestDeviceBatch(ctx, agentID, 1, serverID, []repo.NewOnlineDevice{
		{UserID: userID, NodeID: nodeID, IP: "1.1.1.1", Online: 1},
		{UserID: userID, NodeID: nodeID, IP: "2.2.2.2", Online: 2},
	}, map[int64]int{userID: 2}, now.Unix()); err != nil {
		t.Fatalf("ingest devices: %v", err)
	}

	devices, err := r.ListDevicesByNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	later := now.Add(time.Minute)
	if _, _, err := r.IngestDeviceBatch(ctx, agentID, 2, serverID, []repo.NewOnlineDevice{
		{UserID: userID, NodeID: nodeID, IP: "3.3.3.3", Online: 1},
	}, map[int64]int{userID: 1}, later.Unix()); err != nil {
		t.Fatalf("ingest devices: %v", err)
	}
	devices, _ = r.ListDevicesByUser(ctx, userID)
	if len(devices) != 1 || devices[0].IP != "3.3.3.3" {
		t.Fatalf("expected snapshot replaced, got %+v", devices)
	}

	deleted, err := r.DeleteStaleDevicesBatch(ctx, later.Add(time.Minute), 500)
	if err != nil || deleted != 1 {
		t.Fatalf("expected 1 stale deleted, got %d %v", deleted, err)
	}
}

func TestDevicesAndTraffic(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	userID := mustCreateUser(t, r, "uuid-t")
	serverID := mustCreateServer(t, r, "s1")
	nodeID := mustCreateNode(t, r, serverID, "n1", 443)
	agentID, err := r.CreateAgent(ctx, serverID, "agent-hash", "")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	seen := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if _, _, err := r.IngestDeviceBatch(ctx, agentID, 1, serverID, []repo.NewOnlineDevice{
		{UserID: userID, NodeID: nodeID, IP: "1.1.1.1", Online: 1},
		{UserID: userID, NodeID: nodeID, IP: "2.2.2.2", Online: 1},
	}, map[int64]int{userID: 2}, seen.Unix()); err != nil {
		t.Fatalf("ingest devices: %v", err)
	}
	devices, err := r.ListDevicesByUser(ctx, userID)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	records := []repo.NewTrafficRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, U: 10, D: 20, CreatedAt: seen},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, U: 30, D: 40, CreatedAt: seen},
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
	if u.UsedBytes() != 100 {
		t.Fatalf("expected used 100, got %d", u.UsedBytes())
	}

	cutoff := time.Now().Add(time.Minute)
	recDeleted, err := r.DeleteTrafficRecordsBefore(ctx, cutoff)
	if err != nil || recDeleted != 2 {
		t.Fatalf("delete records: %d %v", recDeleted, err)
	}
}

func TestIngestDeviceBatchChunkedSnapshot(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	serverID := mustCreateServer(t, r, "s1")
	nodeID := mustCreateNode(t, r, serverID, "n1", 443)
	userA := mustCreateUser(t, r, "uuid-a")
	userB := mustCreateUser(t, r, "uuid-b")
	agentID, err := r.CreateAgent(ctx, serverID, "agent-hash", "")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	snapshotAt := time.Now().UTC().Truncate(time.Second)
	if _, dup, err := r.IngestDeviceBatch(ctx, agentID, 1, serverID,
		[]repo.NewOnlineDevice{{UserID: userA, NodeID: nodeID, IP: "1.1.1.1"}},
		map[int64]int{userA: 1}, snapshotAt.Unix()); err != nil || dup {
		t.Fatalf("first chunk: dup=%v err=%v", dup, err)
	}
	if _, dup, err := r.IngestDeviceBatch(ctx, agentID, 2, serverID,
		[]repo.NewOnlineDevice{{UserID: userB, NodeID: nodeID, IP: "2.2.2.2"}},
		map[int64]int{userB: 1}, snapshotAt.Unix()); err != nil || dup {
		t.Fatalf("second chunk: dup=%v err=%v", dup, err)
	}

	devices, err := r.ListDevicesByNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("chunks sharing recorded_at must both survive, got %+v", devices)
	}

	later := snapshotAt.Add(time.Minute)
	if _, _, err := r.IngestDeviceBatch(ctx, agentID, 3, serverID,
		[]repo.NewOnlineDevice{{UserID: userB, NodeID: nodeID, IP: "2.2.2.2"}},
		map[int64]int{userB: 1}, later.Unix()); err != nil {
		t.Fatalf("later snapshot: %v", err)
	}
	devices, err = r.ListDevicesByNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 1 || devices[0].UserID != userB {
		t.Fatalf("devices absent from a later snapshot must be pruned, got %+v", devices)
	}

	ua, _ := r.GetUser(ctx, userA)
	ub, _ := r.GetUser(ctx, userB)
	if ua.OnlineCount != 0 || ub.OnlineCount != 1 {
		t.Fatalf("online_count must track current rows, got A=%d B=%d", ua.OnlineCount, ub.OnlineCount)
	}
}

func TestIngestTrafficBatchAppliesNodeRate(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	serverID := mustCreateServer(t, r, "s1")
	nodeSingle, err := r.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: "n1", Protocol: repo.ProtocolVLESS, Port: 443, Rate: 1})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	nodeDouble, err := r.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: "n2", Protocol: repo.ProtocolVLESS, Port: 8443, Rate: 2})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	nodeFractional, err := r.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: "n3", Protocol: repo.ProtocolVLESS, Port: 8444, Rate: 1.5})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	userID := mustCreateUser(t, r, "uuid-rate")
	agentID, err := r.CreateAgent(ctx, serverID, "agent-hash", "")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	seen := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	records := []repo.NewTrafficRecord{
		{UserID: userID, NodeID: nodeSingle, ServerID: serverID, U: 10, D: 20, CreatedAt: seen},
		{UserID: userID, NodeID: nodeDouble, ServerID: serverID, U: 10, D: 20, CreatedAt: seen},
		{UserID: userID, NodeID: nodeFractional, ServerID: serverID, U: 3, D: 3, CreatedAt: seen},
	}
	if _, dup, err := r.IngestTrafficBatch(ctx, agentID, 1, records); err != nil || dup {
		t.Fatalf("ingest: dup=%v err=%v", dup, err)
	}

	upload, download, err := r.SumTraffic(ctx, repo.TrafficFilter{UserID: userID})
	if err != nil || upload != 35 || download != 65 {
		t.Fatalf("rate-scaled records: up=%d down=%d err=%v", upload, download, err)
	}
	u, _ := r.GetUser(ctx, userID)
	if u.U != 35 || u.D != 65 {
		t.Fatalf("rate-scaled user totals: u=%d d=%d", u.U, u.D)
	}

	if _, dup, err := r.IngestTrafficBatch(ctx, agentID, 1, records); err != nil || !dup {
		t.Fatalf("duplicate must be reported: dup=%v err=%v", dup, err)
	}
	u, _ = r.GetUser(ctx, userID)
	if u.U != 35 || u.D != 65 {
		t.Fatalf("duplicate must not double-apply: u=%d d=%d", u.U, u.D)
	}
}

func TestIngestTrafficBatchRateOverflowIsClamped(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	serverID := mustCreateServer(t, r, "s1")
	nodeID, err := r.CreateNode(ctx, repo.NewNode{
		ServerID: serverID, Name: "huge", Protocol: repo.ProtocolVLESS, Port: 443, Rate: 1e18,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	userID := mustCreateUser(t, r, "uuid-overflow")
	agentID, err := r.CreateAgent(ctx, serverID, "agent-hash", "")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	seen := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)
	if _, _, err := r.IngestTrafficBatch(ctx, agentID, 1, []repo.NewTrafficRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, U: 10, D: 0, CreatedAt: seen},
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	u, _ := r.GetUser(ctx, userID)
	if u.U < 0 || u.D < 0 {
		t.Fatalf("rate overflow must not wrap into negatives: u=%d d=%d", u.U, u.D)
	}
	if u.U != math.MaxInt64 {
		t.Fatalf("rate overflow must clamp to MaxInt64, got %d", u.U)
	}
}

func TestOnlineCountCountsDistinctIPs(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	serverID := mustCreateServer(t, r, "s1")
	node1 := mustCreateNode(t, r, serverID, "n1", 443)
	node2 := mustCreateNode(t, r, serverID, "n2", 8443)
	userID := mustCreateUser(t, r, "uuid-oc")
	agentID, err := r.CreateAgent(ctx, serverID, "agent-hash", "")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	seen := time.Now().UTC().Truncate(time.Second)
	if _, _, err := r.IngestDeviceBatch(ctx, agentID, 1, serverID, []repo.NewOnlineDevice{
		{UserID: userID, NodeID: node1, IP: "1.1.1.1", Online: 2},
		{UserID: userID, NodeID: node2, IP: "1.1.1.1", Online: 3},
	}, map[int64]int{userID: 2}, seen.Unix()); err != nil {
		t.Fatalf("ingest devices: %v", err)
	}
	u, _ := r.GetUser(ctx, userID)
	if u.OnlineCount != 1 {
		t.Fatalf("same IP on two nodes must count once, got %d", u.OnlineCount)
	}
	total, err := r.CountOnlineDevices(ctx)
	if err != nil || total != 1 {
		t.Fatalf("same (user, ip) across nodes must count once globally, got %d %v", total, err)
	}
	devices, err := r.ListDevicesByUser(ctx, userID)
	if err != nil || len(devices) != 2 {
		t.Fatalf("list devices: %d %v", len(devices), err)
	}
	onlineByNode := map[int64]int{}
	for _, d := range devices {
		onlineByNode[d.NodeID] = d.Online
	}
	if onlineByNode[node1] != 2 || onlineByNode[node2] != 3 {
		t.Fatalf("online must persist per (user, node), got %+v", onlineByNode)
	}

	if _, _, err := r.IngestDeviceBatch(ctx, agentID, 2, serverID, []repo.NewOnlineDevice{
		{UserID: userID, NodeID: node1, IP: "1.1.1.1", Online: 4},
		{UserID: userID, NodeID: node1, IP: "2.2.2.2", Online: 4},
	}, map[int64]int{userID: 2}, seen.Add(time.Second).Unix()); err != nil {
		t.Fatalf("ingest devices: %v", err)
	}
	u, _ = r.GetUser(ctx, userID)
	if u.OnlineCount != 2 {
		t.Fatalf("two distinct IPs must count twice, got %d", u.OnlineCount)
	}
	total, err = r.CountOnlineDevices(ctx)
	if err != nil || total != 2 {
		t.Fatalf("two distinct (user, ip) pairs must count twice globally, got %d %v", total, err)
	}
	devices, err = r.ListDevicesByUser(ctx, userID)
	if err != nil || len(devices) != 2 {
		t.Fatalf("list devices: %d %v", len(devices), err)
	}
	for _, d := range devices {
		if d.Online != 4 {
			t.Fatalf("online must refresh on upsert, got %+v", d)
		}
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

	exists, _ = r.DeviceBatchExists(ctx, agentID, 7)
	if exists {
		t.Fatal("expected device batch absent")
	}
	if err := r.RecordDeviceBatch(ctx, agentID, 7); err != nil {
		t.Fatalf("record device batch: %v", err)
	}
	exists, _ = r.DeviceBatchExists(ctx, agentID, 7)
	if !exists {
		t.Fatal("expected device batch present")
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
