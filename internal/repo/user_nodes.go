package repo

import (
	"context"
	"database/sql"
	"sort"
)

func (r *Repo) SetUserNodes(ctx context.Context, userID int64, nodeIDs []int64) error {
	unique := dedupeInt64(nodeIDs)
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_nodes WHERE user_id = ?`, userID); err != nil {
			return mapErr(err)
		}
		now := nowUnix()
		for _, nodeID := range unique {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (?, ?, ?)`,
				userID, nodeID, now); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func (r *Repo) AuthorizeUserNode(ctx context.Context, userID, nodeID int64) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (?, ?, ?)
		 ON CONFLICT (user_id, node_id) DO NOTHING`,
		userID, nodeID, nowUnix())
	return mapErr(err)
}

func (r *Repo) RevokeUserNode(ctx context.Context, userID, nodeID int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM user_nodes WHERE user_id = ? AND node_id = ?`, userID, nodeID)
	return mapErr(err)
}

func (r *Repo) ListNodeIDsByUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT node_id FROM user_nodes WHERE user_id = ? ORDER BY node_id`, userID)
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

func (r *Repo) ListUserIDsByNode(ctx context.Context, nodeID int64) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT user_id FROM user_nodes WHERE node_id = ? ORDER BY user_id`, nodeID)
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

func (r *Repo) ListNodeIDsByServerWithUsers(ctx context.Context, serverID int64) ([]int64, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT DISTINCT un.node_id FROM user_nodes un
		 JOIN nodes n ON n.id = un.node_id
		 WHERE n.server_id = ? ORDER BY un.node_id`, serverID)
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

func (r *Repo) CountNodesByUser(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_nodes WHERE user_id = ?`, userID).Scan(&n)
	return n, mapErr(err)
}

func (r *Repo) CountUsersByNode(ctx context.Context, nodeID int64) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_nodes WHERE node_id = ?`, nodeID).Scan(&n)
	return n, mapErr(err)
}

func dedupeInt64(values []int64) []int64 {
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	out := make([]int64, 0, len(sorted))
	for i, v := range sorted {
		if i > 0 && v == sorted[i-1] {
			continue
		}
		out = append(out, v)
	}
	return out
}
