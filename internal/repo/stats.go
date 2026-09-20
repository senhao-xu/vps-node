package repo

import (
	"context"
	"time"
)

func (r *Repo) CountUsersTotal(ctx context.Context) (int64, error) {
	return r.CountUsers(ctx)
}

func (r *Repo) CountUsersWithFreshSession(ctx context.Context, within time.Time) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT user_id) FROM sessions WHERE last_seen_at >= ?`, within.Unix()).Scan(&n)
	return n, mapErr(err)
}

func (r *Repo) CountServersTotal(ctx context.Context) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers`).Scan(&n)
	return n, mapErr(err)
}

func (r *Repo) CountServersHealthy(ctx context.Context, within time.Time) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM servers WHERE status = 'active' AND last_seen_at IS NOT NULL AND last_seen_at >= ?`,
		within.Unix()).Scan(&n)
	return n, mapErr(err)
}

func (r *Repo) SumTrafficSince(ctx context.Context, since time.Time) (upload, download int64, err error) {
	err = r.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
		 FROM traffic_records WHERE created_at >= ?`, since.Unix()).Scan(&upload, &download)
	return upload, download, mapErr(err)
}
