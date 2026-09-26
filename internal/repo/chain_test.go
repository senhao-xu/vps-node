package repo_test

import (
	"context"
	"errors"
	"testing"

	"vps-node/internal/repo"
)

func mustCreateNodeWithChain(t *testing.T, r *repo.Repo, serverID int64, name string, port int, chainNodeID *int64) int64 {
	t.Helper()
	id, err := r.CreateNode(context.Background(), repo.NewNode{
		ServerID: serverID, Address: name + ".example.com", Name: name, Protocol: repo.ProtocolShadowsocks, Port: port,
		ProtocolSettings: `{"cipher":"2022-blake3-aes-128-gcm"}`,
		ChainNodeID:      chainNodeID,
	})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	return id
}

func TestValidateNodeChain(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	server := mustCreateServer(t, r, "s1")

	a := mustCreateNode(t, r, server, "a", 1001)
	b := mustCreateNodeWithChain(t, r, server, "b", 1002, &a)
	c := mustCreateNodeWithChain(t, r, server, "c", 1003, &b)

	// New node chaining to any existing node is fine.
	if _, err := r.ValidateNodeChain(ctx, 0, c); err != nil {
		t.Fatalf("fresh chain must validate: %v", err)
	}
	// Self reference is a cycle.
	if _, err := r.ValidateNodeChain(ctx, a, a); !errors.Is(err, repo.ErrChainCycle) {
		t.Fatalf("self chain must be a cycle, got %v", err)
	}
	// Multi-level cycle: c→b→a exists, so chaining a→c closes a loop.
	if _, err := r.ValidateNodeChain(ctx, a, c); !errors.Is(err, repo.ErrChainCycle) {
		t.Fatalf("multi-level cycle must be rejected, got %v", err)
	}
	// Direct 2-cycle: b→a exists, so a→b must be rejected.
	if _, err := r.ValidateNodeChain(ctx, a, b); !errors.Is(err, repo.ErrChainCycle) {
		t.Fatalf("two-node cycle must be rejected, got %v", err)
	}
	// Extending the chain forward is fine (c has no downstream loop through d).
	d := mustCreateNode(t, r, server, "d", 1004)
	if _, err := r.ValidateNodeChain(ctx, d, c); err != nil {
		t.Fatalf("extending the chain must validate: %v", err)
	}
	// Missing target surfaces ErrNotFound for the handler to map.
	if _, err := r.ValidateNodeChain(ctx, 0, 9999); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("missing target must be ErrNotFound, got %v", err)
	}
}

func TestChainRevisionDualBump(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	serverA := mustCreateServer(t, r, "entry")
	serverB := mustCreateServer(t, r, "exit")

	revA0, _ := r.GetServerRevision(ctx, serverA)
	revB0, _ := r.GetServerRevision(ctx, serverB)

	exitNode, err := r.CreateNodeAndBump(ctx, repo.NewNode{
		ServerID: serverB, Address: "exit.example.com", Name: "exit", Protocol: repo.ProtocolShadowsocks, Port: 2001,
		ProtocolSettings: `{"cipher":"2022-blake3-aes-128-gcm"}`,
	})
	if err != nil {
		t.Fatalf("create exit node: %v", err)
	}
	revB1, _ := r.GetServerRevision(ctx, serverB)
	if revB1 == revB0 {
		t.Fatal("plain node create must bump its server")
	}

	entryNode, err := r.CreateNodeAndBump(ctx, repo.NewNode{
		ServerID: serverA, Address: "entry.example.com", Name: "entry", Protocol: repo.ProtocolShadowsocks, Port: 1001,
		ProtocolSettings: `{"cipher":"2022-blake3-aes-128-gcm"}`,
		ChainNodeID:      &exitNode,
	})
	if err != nil {
		t.Fatalf("create entry node: %v", err)
	}

	// Create with a chain bumps both servers.
	revA1, _ := r.GetServerRevision(ctx, serverA)
	revB2, _ := r.GetServerRevision(ctx, serverB)
	if revA1 == revA0 {
		t.Fatal("create with chain must bump the entry server")
	}
	if revB2 == revB1 {
		t.Fatal("create with chain must bump the exit server")
	}

	// Updating the exit node bumps the entry server too (its outbound changes).
	if err := r.UpdateNodeAndBump(ctx, exitNode, serverB, "exit.example.com", "exit", "", false, 2001, `{"cipher":"2022-blake3-aes-128-gcm"}`, nil, 1, "[]", nil, nil); err != nil {
		t.Fatalf("update exit node: %v", err)
	}
	revA2, _ := r.GetServerRevision(ctx, serverA)
	if revA2 == revA1 {
		t.Fatal("exit node update must bump the referencing entry server revision")
	}

	// Relinking the entry node bumps the new exit's server as well.
	serverC := mustCreateServer(t, r, "exit-2")
	revC0, _ := r.GetServerRevision(ctx, serverC)
	exit2 := mustCreateNode(t, r, serverC, "exit2", 3001)
	revBBefore, _ := r.GetServerRevision(ctx, serverB)
	if err := r.UpdateNodeAndBump(ctx, entryNode, serverA, "entry.example.com", "entry", "", false, 1001, `{"cipher":"2022-blake3-aes-128-gcm"}`, nil, 1, "[]", &exit2, nil); err != nil {
		t.Fatalf("relink entry node: %v", err)
	}
	revC1, _ := r.GetServerRevision(ctx, serverC)
	if revC1 == revC0 {
		t.Fatal("relinking must bump the new exit server revision")
	}
	revBAfter, _ := r.GetServerRevision(ctx, serverB)
	if revBAfter == revBBefore {
		t.Fatal("relinking must bump the old exit server revision")
	}

	// Deleting a chained entry node bumps the exit server (relay user removed).
	revC2, _ := r.GetServerRevision(ctx, serverC)
	if err := r.DeleteNodeAndBump(ctx, entryNode, serverA); err != nil {
		t.Fatalf("delete entry node: %v", err)
	}
	revC3, _ := r.GetServerRevision(ctx, serverC)
	if revC3 == revC2 {
		t.Fatal("deleting a chained entry node must bump the exit server revision")
	}
}

func TestListNodesByChainTarget(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	serverA := mustCreateServer(t, r, "a")
	serverB := mustCreateServer(t, r, "b")

	exit := mustCreateNode(t, r, serverB, "exit", 2001)
	entry1 := mustCreateNodeWithChain(t, r, serverA, "e1", 1001, &exit)
	entry2 := mustCreateNodeWithChain(t, r, serverB, "e2", 1002, &exit)
	mustCreateNode(t, r, serverA, "plain", 1003)

	refs, err := r.ListNodesByChainTarget(ctx, exit)
	if err != nil {
		t.Fatalf("list referencing: %v", err)
	}
	if len(refs) != 2 || refs[0].ID != entry1 || refs[1].ID != entry2 {
		t.Fatalf("unexpected referencing nodes: %+v", refs)
	}

	// Exit-side lookup: entries across servers targeting serverB's nodes.
	entries, err := r.ListChainEntriesTargetingServer(ctx, serverB)
	if err != nil {
		t.Fatalf("list entries targeting server: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries targeting server B, got %+v", entries)
	}
	entries, err = r.ListChainEntriesTargetingServer(ctx, serverA)
	if err != nil || len(entries) != 0 {
		t.Fatalf("no node chains into server A, got %+v err=%v", entries, err)
	}
}

func TestDeleteServerCascadeUnlinksChainReferences(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	serverA := mustCreateServer(t, r, "entry")
	serverB := mustCreateServer(t, r, "exit")

	exit := mustCreateNode(t, r, serverB, "exit", 2001)
	entry := mustCreateNodeWithChain(t, r, serverA, "entry", 1001, &exit)

	revBefore, _ := r.GetServerRevision(ctx, serverA)
	if err := r.DeleteServerCascade(ctx, serverB); err != nil {
		t.Fatalf("server delete must not be blocked by chain references: %v", err)
	}
	revAfter, _ := r.GetServerRevision(ctx, serverA)
	if revAfter == revBefore {
		t.Fatal("deleting an exit server must bump the referencing entry server")
	}
	n, err := r.GetNode(ctx, entry)
	if err != nil {
		t.Fatalf("entry node must survive the exit server deletion: %v", err)
	}
	if n.ChainNodeID != nil {
		t.Fatalf("chain must be unlinked after exit server deletion, got %v", *n.ChainNodeID)
	}
}

func TestChainNodeColumns(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	server := mustCreateServer(t, r, "s1")

	exit := mustCreateNode(t, r, server, "exit", 2001)
	entry := mustCreateNodeWithChain(t, r, server, "entry", 1001, &exit)

	plain, err := r.GetNode(ctx, entry)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if plain.ChainNodeID == nil || *plain.ChainNodeID != exit {
		t.Fatalf("chain_node_id must round-trip, got %+v", plain.ChainNodeID)
	}

	joined, err := r.ListNodesByServer(ctx, server)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	byID := map[int64]repo.Node{}
	for _, n := range joined {
		byID[n.ID] = n
	}
	entryJoined := byID[entry]
	if entryJoined.ChainNodeName != "exit" || entryJoined.ChainServerName != "s1" {
		t.Fatalf("joined select must populate chain display names, got %+v", entryJoined)
	}
	exitJoined := byID[exit]
	if exitJoined.ChainNodeID != nil || exitJoined.ChainNodeName != "" {
		t.Fatalf("unlinked node must have empty chain fields, got %+v", exitJoined)
	}
}
