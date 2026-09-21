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

type UserTrafficSum struct {
	UserID        int64
	UploadBytes   int64
	DownloadBytes int64
}

func (r *Repo) SumTrafficByUser(ctx context.Context, f TrafficFilter) ([]UserTrafficSum, error) {
	whereSQL, args := trafficWhere(f)
	rows, err := r.DB.QueryContext(ctx,
		`SELECT user_id, COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
		 FROM traffic_records WHERE `+whereSQL+` GROUP BY user_id ORDER BY user_id`, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	sums := []UserTrafficSum{}
	for rows.Next() {
		var s UserTrafficSum
		if err := rows.Scan(&s.UserID, &s.UploadBytes, &s.DownloadBytes); err != nil {
			return nil, mapErr(err)
		}
		sums = append(sums, s)
	}
	return sums, rows.Err()
}

type UserNodeTrafficSum struct {
	UserID        int64
	NodeID        int64
	NodeName      string
	ServerID      int64
	ServerName    string
	UploadBytes   int64
	DownloadBytes int64
}

func (r *Repo) SumTrafficByUserNode(ctx context.Context, f TrafficFilter) ([]UserNodeTrafficSum, error) {
	whereSQL, args := trafficWherePrefixed(f, "tr")
	rows, err := r.DB.QueryContext(ctx,
		`SELECT tr.user_id, tr.node_id, n.name, n.server_id, s.name,
		        COALESCE(SUM(tr.upload_bytes), 0), COALESCE(SUM(tr.download_bytes), 0)
		 FROM traffic_records tr
		 JOIN nodes n ON n.id = tr.node_id
		 JOIN servers s ON s.id = n.server_id
		 WHERE `+whereSQL+` GROUP BY tr.user_id, tr.node_id ORDER BY tr.user_id, tr.node_id`, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	sums := []UserNodeTrafficSum{}
	for rows.Next() {
		var s UserNodeTrafficSum
		if err := rows.Scan(&s.UserID, &s.NodeID, &s.NodeName, &s.ServerID, &s.ServerName,
			&s.UploadBytes, &s.DownloadBytes); err != nil {
			return nil, mapErr(err)
		}
		sums = append(sums, s)
	}
	return sums, rows.Err()
}
