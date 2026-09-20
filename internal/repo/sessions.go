package repo

import (
	"context"
	"database/sql"
	"time"
)

type Session struct {
	ID            int64
	UserID        int64
	NodeID        int64
	ServerID      int64
	IP            string
	UploadBytes   int64
	DownloadBytes int64
	ConnectedAt   time.Time
	LastSeenAt    time.Time
}

type NewSession struct {
	UserID        int64
	NodeID        int64
	ServerID      int64
	IP            string
	UploadBytes   int64
	DownloadBytes int64
	ConnectedAt   time.Time
	LastSeenAt    time.Time
}

func (r *Repo) ReplaceServerSessions(ctx context.Context, serverID int64, sessions []NewSession) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE server_id = ?`, serverID); err != nil {
			return mapErr(err)
		}
		for _, s := range sessions {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO sessions (user_id, node_id, server_id, ip, upload_bytes, download_bytes, connected_at, last_seen_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				s.UserID, s.NodeID, s.ServerID, s.IP, s.UploadBytes, s.DownloadBytes, s.ConnectedAt.Unix(), s.LastSeenAt.Unix()); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func (r *Repo) ListSessionsByUser(ctx context.Context, userID int64, activeWithin time.Time) ([]Session, error) {
	rows, err := r.DB.QueryContext(ctx, sessionSelect+` WHERE user_id = ? AND last_seen_at >= ? ORDER BY last_seen_at DESC`, userID, activeWithin.Unix())
	if err != nil {
		return nil, mapErr(err)
	}
	return collectSessions(rows)
}

func (r *Repo) ListSessionsByNode(ctx context.Context, nodeID int64, activeWithin time.Time) ([]Session, error) {
	rows, err := r.DB.QueryContext(ctx, sessionSelect+` WHERE node_id = ? AND last_seen_at >= ? ORDER BY last_seen_at DESC`, nodeID, activeWithin.Unix())
	if err != nil {
		return nil, mapErr(err)
	}
	return collectSessions(rows)
}

func (r *Repo) ListSessionsByServer(ctx context.Context, serverID int64, activeWithin time.Time) ([]Session, error) {
	rows, err := r.DB.QueryContext(ctx, sessionSelect+` WHERE server_id = ? AND last_seen_at >= ? ORDER BY last_seen_at DESC`, serverID, activeWithin.Unix())
	if err != nil {
		return nil, mapErr(err)
	}
	return collectSessions(rows)
}

func (r *Repo) CountActiveSessions(ctx context.Context, activeWithin time.Time) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE last_seen_at >= ?`, activeWithin.Unix()).Scan(&n)
	return n, mapErr(err)
}

func (r *Repo) DeleteSessionsLastSeenBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM sessions WHERE last_seen_at < ?`, cutoff.Unix())
	if err != nil {
		return 0, mapErr(err)
	}
	return res.RowsAffected()
}

const sessionSelect = `SELECT id, user_id, node_id, server_id, ip, upload_bytes, download_bytes, connected_at, last_seen_at FROM sessions`

func collectSessions(rows *sql.Rows) ([]Session, error) {
	defer rows.Close()
	sessions := []Session{}
	for rows.Next() {
		var s Session
		var connectedAt, lastSeenAt int64
		if err := rows.Scan(&s.ID, &s.UserID, &s.NodeID, &s.ServerID, &s.IP, &s.UploadBytes, &s.DownloadBytes,
			&connectedAt, &lastSeenAt); err != nil {
			return nil, mapErr(err)
		}
		s.ConnectedAt = toTime(connectedAt)
		s.LastSeenAt = toTime(lastSeenAt)
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}
