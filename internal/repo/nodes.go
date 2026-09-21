package repo

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type Node struct {
	ID         int64
	ServerID   int64
	Name       string
	Protocol   string
	Port       int
	Settings   string
	SecretEnc  []byte
	Status     string
	ServerName string
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	ServerID  int64
	Name      string
	Protocol  string
	Port      int
	Settings  string
	SecretEnc []byte
	Status    string
}

const (
	NodeStatusActive   = "active"
	NodeStatusDisabled = "disabled"

	ProtocolShadowsocks = "shadowsocks"
	ProtocolVLESS       = "vless"
	ProtocolHysteria2   = "hysteria2"
	ProtocolAnyTLS      = "anytls"
)

const nodeSelect = `SELECT id, server_id, name, protocol, port, settings, secret_enc, status, created_at, updated_at FROM nodes`

const nodeSelectWithServer = `SELECT n.id, n.server_id, n.name, n.protocol, n.port, n.settings, n.secret_enc, n.status, n.created_at, n.updated_at, s.name
	FROM nodes n JOIN servers s ON s.id = n.server_id`

func insertNodeExec(ctx context.Context, q execer, n NewNode) (int64, error) {
	if n.Status == "" {
		n.Status = NodeStatusActive
	}
	if n.Settings == "" {
		n.Settings = "{}"
	}
	now := nowUnix()
	res, err := q.ExecContext(ctx,
		`INSERT INTO nodes (server_id, name, protocol, port, settings, secret_enc, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ServerID, n.Name, n.Protocol, n.Port, n.Settings, n.SecretEnc, n.Status, now, now)
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

func (r *Repo) UpdateNodeSpec(ctx context.Context, id int64, name string, port int, settings string, secretEnc []byte) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE nodes SET name = ?, port = ?, settings = ?, secret_enc = ?, updated_at = ? WHERE id = ?`,
		name, port, settings, secretEnc, nowUnix(), id)
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

func scanNode(scan func(dest ...any) error) (Node, error) {
	var n Node
	var secretEnc []byte
	var createdAt, updatedAt int64
	err := scan(&n.ID, &n.ServerID, &n.Name, &n.Protocol, &n.Port, &n.Settings, &secretEnc, &n.Status, &createdAt, &updatedAt)
	if err != nil {
		return Node{}, mapErr(err)
	}
	n.SecretEnc = secretEnc
	n.CreatedAt = toTime(createdAt)
	n.UpdatedAt = toTime(updatedAt)
	return n, nil
}

func scanNodeWithServerName(scan func(dest ...any) error) (Node, error) {
	var n Node
	var secretEnc []byte
	var createdAt, updatedAt int64
	err := scan(&n.ID, &n.ServerID, &n.Name, &n.Protocol, &n.Port, &n.Settings, &secretEnc, &n.Status, &createdAt, &updatedAt, &n.ServerName)
	if err != nil {
		return Node{}, mapErr(err)
	}
	n.SecretEnc = secretEnc
	n.CreatedAt = toTime(createdAt)
	n.UpdatedAt = toTime(updatedAt)
	return n, nil
}
