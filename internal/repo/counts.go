package repo

import (
	"context"
	"strings"
	"time"
)

func inPlaceholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func int64Args(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}

func (r *Repo) CountNodesByUserIDs(ctx context.Context, userIDs []int64) (map[int64]int64, error) {
	if len(userIDs) == 0 {
		return map[int64]int64{}, nil
	}
	query := `SELECT user_id, COUNT(*) FROM user_nodes WHERE user_id IN (` + inPlaceholders(len(userIDs)) + `) GROUP BY user_id`
	return r.groupedCount(ctx, query, int64Args(userIDs))
}

func (r *Repo) CountFreshSessionsByUserIDs(ctx context.Context, userIDs []int64, within time.Time) (map[int64]int64, error) {
	if len(userIDs) == 0 {
		return map[int64]int64{}, nil
	}
	query := `SELECT user_id, COUNT(*) FROM sessions WHERE last_seen_at >= ? AND user_id IN (` + inPlaceholders(len(userIDs)) + `) GROUP BY user_id`
	args := append([]any{within.Unix()}, int64Args(userIDs)...)
	return r.groupedCount(ctx, query, args)
}

func (r *Repo) CountNodesByServerIDs(ctx context.Context, serverIDs []int64) (map[int64]int64, error) {
	if len(serverIDs) == 0 {
		return map[int64]int64{}, nil
	}
	query := `SELECT server_id, COUNT(*) FROM nodes WHERE server_id IN (` + inPlaceholders(len(serverIDs)) + `) GROUP BY server_id`
	return r.groupedCount(ctx, query, int64Args(serverIDs))
}

func (r *Repo) CountFreshUsersByServerIDs(ctx context.Context, serverIDs []int64, within time.Time) (map[int64]int64, error) {
	if len(serverIDs) == 0 {
		return map[int64]int64{}, nil
	}
	query := `SELECT server_id, COUNT(DISTINCT user_id) FROM sessions WHERE last_seen_at >= ? AND server_id IN (` + inPlaceholders(len(serverIDs)) + `) GROUP BY server_id`
	args := append([]any{within.Unix()}, int64Args(serverIDs)...)
	return r.groupedCount(ctx, query, args)
}

func (r *Repo) CountUsersByNodeIDs(ctx context.Context, nodeIDs []int64) (map[int64]int64, error) {
	if len(nodeIDs) == 0 {
		return map[int64]int64{}, nil
	}
	query := `SELECT node_id, COUNT(*) FROM user_nodes WHERE node_id IN (` + inPlaceholders(len(nodeIDs)) + `) GROUP BY node_id`
	return r.groupedCount(ctx, query, int64Args(nodeIDs))
}

func (r *Repo) CountFreshSessionsByNodeIDs(ctx context.Context, nodeIDs []int64, within time.Time) (map[int64]int64, error) {
	if len(nodeIDs) == 0 {
		return map[int64]int64{}, nil
	}
	query := `SELECT node_id, COUNT(*) FROM sessions WHERE last_seen_at >= ? AND node_id IN (` + inPlaceholders(len(nodeIDs)) + `) GROUP BY node_id`
	args := append([]any{within.Unix()}, int64Args(nodeIDs)...)
	return r.groupedCount(ctx, query, args)
}

func (r *Repo) groupedCount(ctx context.Context, query string, args []any) (map[int64]int64, error) {
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	counts := map[int64]int64{}
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, mapErr(err)
		}
		counts[id] = n
	}
	return counts, rows.Err()
}

func (r *Repo) ListServerIDsByUser(ctx context.Context, userID int64) ([]int64, error) {
	return listServerIDsByUserExec(ctx, r.DB, userID)
}
