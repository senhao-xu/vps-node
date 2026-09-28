package repo

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// Custom node sources: administrator-maintained external nodes merged into
// user subscriptions. They belong to no server/agent, never bump server
// revisions, and never participate in traffic or device accounting.

const (
	CustomNodeSourceLinks        = "links"
	CustomNodeSourceSubscription = "subscription"

	CustomNodeStatusActive   = "active"
	CustomNodeStatusDisabled = "disabled"
)

type CustomNode struct {
	ID            int64
	Name          string
	SourceType    string
	UserAgent     string
	ContentEnc    []byte
	CachedContent string
	FetchedAt     time.Time
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type NewCustomNode struct {
	Name       string
	SourceType string
	UserAgent  string
	ContentEnc []byte
	Status     string
}

const customNodeSelect = `SELECT id, name, source_type, content_enc, cached_content, fetched_at, status, created_at, updated_at, user_agent FROM custom_nodes`

func (r *Repo) CreateCustomNode(ctx context.Context, n NewCustomNode) (int64, error) {
	if n.Status == "" {
		n.Status = CustomNodeStatusActive
	}
	now := nowUnix()
	res, err := r.DB.ExecContext(ctx,
		`INSERT INTO custom_nodes (name, source_type, content_enc, status, created_at, updated_at, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.Name, n.SourceType, n.ContentEnc, n.Status, now, now, n.UserAgent)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

func (r *Repo) GetCustomNode(ctx context.Context, id int64) (CustomNode, error) {
	row := r.DB.QueryRowContext(ctx, customNodeSelect+` WHERE id = ?`, id)
	return scanCustomNode(row.Scan)
}

func (r *Repo) ListCustomNodes(ctx context.Context) ([]CustomNode, error) {
	rows, err := r.DB.QueryContext(ctx, customNodeSelect+` ORDER BY id`)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectCustomNodes(rows)
}

func (r *Repo) ListCustomNodesByIDs(ctx context.Context, ids []int64) ([]CustomNode, error) {
	if len(ids) == 0 {
		return []CustomNode{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.DB.QueryContext(ctx, customNodeSelect+` WHERE id IN (`+placeholders+`) ORDER BY id`, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectCustomNodes(rows)
}

// ListActiveCustomNodesByUser returns the active custom nodes authorized for
// the given user, ordered by id so subscription output is stable.
func (r *Repo) ListActiveCustomNodesByUser(ctx context.Context, userID int64) ([]CustomNode, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT c.id, c.name, c.source_type, c.content_enc, c.cached_content, c.fetched_at, c.status, c.created_at, c.updated_at, c.user_agent
		 FROM custom_nodes c
		 JOIN user_custom_nodes ucn ON ucn.custom_node_id = c.id
		 WHERE ucn.user_id = ? AND c.status = ? ORDER BY c.id`, userID, CustomNodeStatusActive)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectCustomNodes(rows)
}

// UpdateCustomNode applies a full edit. name, user_agent and status are always
// updated. When contentEnc is nil the stored content is kept; when it changes
// it is replaced. The fetch cache is invalidated when the content changes or
// the caller explicitly asks for it (e.g. a User-Agent change), so a renamed
// or re-targeted subscription never serves stale upstream data.
func (r *Repo) UpdateCustomNode(ctx context.Context, id int64, name string, contentEnc []byte, status, userAgent string, invalidateCache bool) error {
	sets := []string{"name = ?", "user_agent = ?", "status = ?", "updated_at = ?"}
	args := []any{name, userAgent, status, nowUnix()}
	if contentEnc != nil {
		sets = append(sets, "content_enc = ?")
		args = append(args, contentEnc)
	}
	if contentEnc != nil || invalidateCache {
		sets = append(sets, "cached_content = ''", "fetched_at = 0")
	}
	args = append(args, id)
	res, err := r.DB.ExecContext(ctx,
		`UPDATE custom_nodes SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...)
	if err != nil {
		return mapErr(err)
	}
	if rowsAffected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateCustomNodeCache stores the last successful upstream fetch. Failures
// here must not fail the subscription render that produced the content.
func (r *Repo) UpdateCustomNodeCache(ctx context.Context, id int64, content string, fetchedAt int64) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE custom_nodes SET cached_content = ?, fetched_at = ? WHERE id = ?`,
		content, fetchedAt, id)
	return mapErr(err)
}

func (r *Repo) DeleteCustomNode(ctx context.Context, id int64) error {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM custom_nodes WHERE id = ?`, id)
	if err != nil {
		return mapErr(err)
	}
	if rowsAffected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

// --- user_custom_nodes authorization (mirrors user_nodes.go semantics) ---

func (r *Repo) SetUserCustomNodes(ctx context.Context, userID int64, customNodeIDs []int64) error {
	unique := dedupeInt64(customNodeIDs)
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_custom_nodes WHERE user_id = ?`, userID); err != nil {
			return mapErr(err)
		}
		now := nowUnix()
		for _, customNodeID := range unique {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO user_custom_nodes (user_id, custom_node_id, created_at) VALUES (?, ?, ?)`,
				userID, customNodeID, now); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func (r *Repo) AuthorizeUserCustomNode(ctx context.Context, userID, customNodeID int64) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO user_custom_nodes (user_id, custom_node_id, created_at) VALUES (?, ?, ?)
		 ON CONFLICT (user_id, custom_node_id) DO NOTHING`,
		userID, customNodeID, nowUnix())
	return mapErr(err)
}

func (r *Repo) RevokeUserCustomNode(ctx context.Context, userID, customNodeID int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM user_custom_nodes WHERE user_id = ? AND custom_node_id = ?`, userID, customNodeID)
	return mapErr(err)
}

func (r *Repo) ListCustomNodeIDsByUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT custom_node_id FROM user_custom_nodes WHERE user_id = ? ORDER BY custom_node_id`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapErr(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func collectCustomNodes(rows *sql.Rows) ([]CustomNode, error) {
	defer rows.Close()
	nodes := []CustomNode{}
	for rows.Next() {
		n, err := scanCustomNode(rows.Scan)
		if err != nil {
			return nil, mapErr(err)
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func scanCustomNode(scan func(dest ...any) error) (CustomNode, error) {
	var n CustomNode
	var contentEnc []byte
	var fetchedAt, createdAt, updatedAt int64
	err := scan(&n.ID, &n.Name, &n.SourceType, &contentEnc, &n.CachedContent, &fetchedAt, &n.Status, &createdAt, &updatedAt, &n.UserAgent)
	if err != nil {
		return CustomNode{}, mapErr(err)
	}
	n.ContentEnc = contentEnc
	if fetchedAt > 0 {
		n.FetchedAt = toTime(fetchedAt)
	}
	n.CreatedAt = toTime(createdAt)
	n.UpdatedAt = toTime(updatedAt)
	return n, nil
}
