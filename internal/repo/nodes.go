package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Node struct {
	ID               int64
	ServerID         int64
	Address          string
	IPv6Enabled      bool
	IPv6Address      string
	Name             string
	Protocol         string
	Port             int
	ProtocolSettings string
	Rate             float64
	Tags             string
	SecretEnc        []byte
	Status           string
	ServerName       string
	ChainNodeID      *int64
	// ChainNodeName / ChainServerName are populated by the WithServerName
	// queries for display (chain exit "server/name"); plain selects leave
	// them empty.
	ChainNodeName   string
	ChainServerName string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type NodeFilter struct {
	ServerID int64
	Protocol string
	Status   string
	Query    string
	Page     int
	PageSize int
}

type NewNode struct {
	ServerID         int64
	Address          string
	IPv6Enabled      bool
	IPv6Address      string
	Name             string
	Protocol         string
	Port             int
	ProtocolSettings string
	Rate             float64
	Tags             string
	SecretEnc        []byte
	Status           string
	ChainNodeID      *int64
}

const (
	NodeStatusActive   = "active"
	NodeStatusDisabled = "disabled"

	ProtocolShadowsocks = "shadowsocks"
	ProtocolVLESS       = "vless"
	ProtocolHysteria2   = "hysteria2"
	ProtocolAnyTLS      = "anytls"
)

const nodeSelect = `SELECT id, server_id, address, ipv6_enabled, ipv6_address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, chain_node_id, created_at, updated_at FROM nodes`

const nodeSelectWithServer = `SELECT n.id, n.server_id, n.address, n.ipv6_enabled, n.ipv6_address, n.name, n.protocol, n.port, n.protocol_settings, n.rate, n.tags, n.secret_enc, n.status, n.chain_node_id, n.created_at, n.updated_at, s.name, cn.name, cs.name
	FROM nodes n JOIN servers s ON s.id = n.server_id
	LEFT JOIN nodes cn ON cn.id = n.chain_node_id
	LEFT JOIN servers cs ON cs.id = cn.server_id`

func insertNodeExec(ctx context.Context, q execer, n NewNode) (int64, error) {
	if n.Status == "" {
		n.Status = NodeStatusActive
	}
	if n.ProtocolSettings == "" {
		n.ProtocolSettings = "{}"
	}
	if n.Rate == 0 {
		n.Rate = 1
	}
	if n.Tags == "" {
		n.Tags = "[]"
	}
	now := nowUnix()
	res, err := q.ExecContext(ctx,
		`INSERT INTO nodes (server_id, address, ipv6_enabled, ipv6_address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, chain_node_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ServerID, n.Address, ipv6EnabledInt(n.IPv6Enabled), n.IPv6Address, n.Name, n.Protocol, n.Port, n.ProtocolSettings, n.Rate, n.Tags, n.SecretEnc, n.Status, n.ChainNodeID, now, now)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

func (r *Repo) CreateNode(ctx context.Context, n NewNode) (int64, error) {
	return insertNodeExec(ctx, r.DB, n)
}

func (r *Repo) GetNode(ctx context.Context, id int64) (Node, error) {
	row := r.DB.QueryRowContext(ctx, nodeSelect+` WHERE id = ?`, id)
	return scanNode(row.Scan)
}

func (r *Repo) ListNodes(ctx context.Context) ([]Node, error) {
	rows, err := r.DB.QueryContext(ctx, nodeSelect+` ORDER BY server_id, port`)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectNodes(rows)
}

func (r *Repo) ListNodesPage(ctx context.Context, f NodeFilter) ([]Node, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if f.ServerID > 0 {
		where = append(where, "n.server_id = ?")
		args = append(args, f.ServerID)
	}
	if f.Protocol != "" {
		where = append(where, "n.protocol = ?")
		args = append(args, f.Protocol)
	}
	if f.Status != "" {
		where = append(where, "n.status = ?")
		args = append(args, f.Status)
	}
	if f.Query != "" {
		where = append(where, `n.name LIKE '%' || ? || '%' ESCAPE '\'`)
		args = append(args, escapeLike(f.Query))
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM nodes n JOIN servers s ON s.id = n.server_id WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	page, size := normalizePage(f.Page, f.PageSize)
	rows, err := r.DB.QueryContext(ctx,
		nodeSelectWithServer+` WHERE `+whereSQL+` ORDER BY n.server_id, n.port LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	nodes, err := collectNodesWithServerName(rows)
	if err != nil {
		return nil, 0, err
	}
	return nodes, total, nil
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string {
	return likeEscaper.Replace(s)
}

func (r *Repo) ListNodesByServer(ctx context.Context, serverID int64) ([]Node, error) {
	rows, err := r.DB.QueryContext(ctx, nodeSelectWithServer+` WHERE n.server_id = ? ORDER BY n.port`, serverID)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectNodesWithServerName(rows)
}

func (r *Repo) ListNodesByIDs(ctx context.Context, ids []int64) ([]Node, error) {
	if len(ids) == 0 {
		return []Node{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.DB.QueryContext(ctx, nodeSelectWithServer+` WHERE n.id IN (`+placeholders+`) ORDER BY n.id`, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectNodesWithServerName(rows)
}

func (r *Repo) UpdateNodeSpec(ctx context.Context, id int64, address, name, ipv6Address string, ipv6Enabled bool, port int, settings string, secretEnc []byte, rate float64, tags string) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE nodes SET address = ?, ipv6_enabled = ?, ipv6_address = ?, name = ?, port = ?, protocol_settings = ?, secret_enc = ?, rate = ?, tags = ?, updated_at = ? WHERE id = ?`,
		address, ipv6EnabledInt(ipv6Enabled), ipv6Address, name, port, settings, secretEnc, rate, tags, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) SetNodeStatus(ctx context.Context, id int64, status string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE nodes SET status = ?, updated_at = ? WHERE id = ?`, status, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) DeleteNode(ctx context.Context, id int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	return mapErr(err)
}

// ErrChainCycle is returned when a requested chain_node_id would close a
// chain loop (A→B→A, any depth).
var ErrChainCycle = errors.New("node chain would form a cycle")

// ValidateNodeChain verifies that nodeID may chain to chainNodeID: the target
// must exist and following the chain from the target must never reach
// nodeID. nodeID == 0 means a not-yet-created node (no self/cycle possible).
// It returns the target node so callers can check its status.
func (r *Repo) ValidateNodeChain(ctx context.Context, nodeID, chainNodeID int64) (Node, error) {
	target, err := r.GetNode(ctx, chainNodeID)
	if err != nil {
		return Node{}, err
	}
	if nodeID != 0 && chainNodeID == nodeID {
		return target, ErrChainCycle
	}
	var total int64
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&total); err != nil {
		return target, mapErr(err)
	}
	current := target
	for step := int64(0); step <= total; step++ {
		if current.ChainNodeID == nil {
			return target, nil
		}
		next, err := r.GetNode(ctx, *current.ChainNodeID)
		if err != nil {
			// A dangling reference cannot close a loop on nodeID.
			return target, nil
		}
		if nodeID != 0 && next.ID == nodeID {
			return target, ErrChainCycle
		}
		current = next
	}
	// Walked further than the number of nodes: a pre-existing loop.
	return target, ErrChainCycle
}

// ListNodesByChainTarget returns the nodes chaining to chainNodeID (its
// entry nodes), used for delete protection and exit-side relay injection.
func (r *Repo) ListNodesByChainTarget(ctx context.Context, chainNodeID int64) ([]Node, error) {
	rows, err := r.DB.QueryContext(ctx, nodeSelect+` WHERE chain_node_id = ? ORDER BY id`, chainNodeID)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectNodes(rows)
}

// ListChainEntriesTargetingServer returns the entry nodes (any server) whose
// chain exit is one of the given server's nodes.
func (r *Repo) ListChainEntriesTargetingServer(ctx context.Context, serverID int64) ([]Node, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT e.id, e.server_id, e.address, e.ipv6_enabled, e.ipv6_address, e.name, e.protocol, e.port, e.protocol_settings, e.rate, e.tags, e.secret_enc, e.status, e.chain_node_id, e.created_at, e.updated_at
		 FROM nodes e JOIN nodes x ON x.id = e.chain_node_id
		 WHERE x.server_id = ? ORDER BY e.id`, serverID)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectNodes(rows)
}

func collectNodes(rows *sql.Rows) ([]Node, error) {
	defer rows.Close()
	nodes := []Node{}
	for rows.Next() {
		n, err := scanNode(rows.Scan)
		if err != nil {
			return nil, mapErr(err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func collectNodesWithServerName(rows *sql.Rows) ([]Node, error) {
	defer rows.Close()
	nodes := []Node{}
	for rows.Next() {
		n, err := scanNodeWithServerName(rows.Scan)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func ipv6EnabledInt(enabled bool) int {
	if enabled {
		return 1
	}
	return 0
}

func scanNode(scan func(dest ...any) error) (Node, error) {
	var n Node
	var secretEnc []byte
	var createdAt, updatedAt int64
	var ipv6Enabled int
	var chainNodeID sql.NullInt64
	err := scan(&n.ID, &n.ServerID, &n.Address, &ipv6Enabled, &n.IPv6Address, &n.Name, &n.Protocol, &n.Port, &n.ProtocolSettings, &n.Rate, &n.Tags, &secretEnc, &n.Status, &chainNodeID, &createdAt, &updatedAt)
	if err != nil {
		return Node{}, mapErr(err)
	}
	n.IPv6Enabled = ipv6Enabled != 0
	n.SecretEnc = secretEnc
	if chainNodeID.Valid {
		n.ChainNodeID = &chainNodeID.Int64
	}
	n.CreatedAt = toTime(createdAt)
	n.UpdatedAt = toTime(updatedAt)
	return n, nil
}

func scanNodeWithServerName(scan func(dest ...any) error) (Node, error) {
	var n Node
	var secretEnc []byte
	var createdAt, updatedAt int64
	var ipv6Enabled int
	var chainNodeID sql.NullInt64
	var chainNodeName, chainServerName sql.NullString
	err := scan(&n.ID, &n.ServerID, &n.Address, &ipv6Enabled, &n.IPv6Address, &n.Name, &n.Protocol, &n.Port, &n.ProtocolSettings, &n.Rate, &n.Tags, &secretEnc, &n.Status, &chainNodeID, &createdAt, &updatedAt, &n.ServerName, &chainNodeName, &chainServerName)
	if err != nil {
		return Node{}, mapErr(err)
	}
	n.IPv6Enabled = ipv6Enabled != 0
	n.SecretEnc = secretEnc
	if chainNodeID.Valid {
		n.ChainNodeID = &chainNodeID.Int64
	}
	n.ChainNodeName = chainNodeName.String
	n.ChainServerName = chainServerName.String
	n.CreatedAt = toTime(createdAt)
	n.UpdatedAt = toTime(updatedAt)
	return n, nil
}
