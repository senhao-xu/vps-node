package repo

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type ConnectionLog struct {
	ID            int64
	UserID        int64
	NodeID        int64
	ServerID      int64
	IP            string
	Protocol      string
	UploadBytes   int64
	DownloadBytes int64
	ConnectedAt   time.Time
	ClosedAt      *time.Time
	Status        string
	CreatedAt     time.Time
}

type NewConnectionLog struct {
	UserID        int64
	NodeID        int64
	ServerID      int64
	IP            string
	Protocol      string
	UploadBytes   int64
	DownloadBytes int64
	ConnectedAt   time.Time
	ClosedAt      *time.Time
	Status        string
}

type LogFilter struct {
	UserID   int64
	NodeID   int64
	ServerID int64
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

func (r *Repo) InsertConnectionLogs(ctx context.Context, logs []NewConnectionLog) (int64, error) {
	var inserted int64
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		now := nowUnix()
		for _, l := range logs {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO connection_logs (user_id, node_id, server_id, ip, protocol, upload_bytes, download_bytes, connected_at, closed_at, status, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				l.UserID, l.NodeID, l.ServerID, l.IP, l.Protocol, l.UploadBytes, l.DownloadBytes,
				l.ConnectedAt.Unix(), timeArg(l.ClosedAt), l.Status, now)
			if err != nil {
				return mapErr(err)
			}
			inserted += rowsAffected(res)
		}
		return nil
	})
	return inserted, err
}

func (r *Repo) ListConnectionLogs(ctx context.Context, f LogFilter) ([]ConnectionLog, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if f.UserID != 0 {
		where = append(where, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.NodeID != 0 {
		where = append(where, "node_id = ?")
		args = append(args, f.NodeID)
	}
	if f.ServerID != 0 {
		where = append(where, "server_id = ?")
		args = append(args, f.ServerID)
	}
	if f.From != nil {
		where = append(where, "connected_at >= ?")
		args = append(args, f.From.Unix())
	}
	if f.To != nil {
		where = append(where, "connected_at < ?")
		args = append(args, f.To.Unix())
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM connection_logs WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}

	page, size := normalizePage(f.Page, f.PageSize)
	rows, err := r.DB.QueryContext(ctx,
		logSelect+` WHERE `+whereSQL+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	logs, err := collectConnectionLogs(rows)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (r *Repo) DeleteConnectionLogsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM connection_logs WHERE created_at < ?`, cutoff.Unix())
	if err != nil {
		return 0, mapErr(err)
	}
	return res.RowsAffected()
}

const logSelect = `SELECT id, user_id, node_id, server_id, ip, protocol, upload_bytes, download_bytes, connected_at, closed_at, status, created_at FROM connection_logs`

func collectConnectionLogs(rows *sql.Rows) ([]ConnectionLog, error) {
	defer rows.Close()
	logs := []ConnectionLog{}
	for rows.Next() {
		var l ConnectionLog
		var closedAt sql.NullInt64
		var connectedAt, createdAt int64
		if err := rows.Scan(&l.ID, &l.UserID, &l.NodeID, &l.ServerID, &l.IP, &l.Protocol, &l.UploadBytes, &l.DownloadBytes,
			&connectedAt, &closedAt, &l.Status, &createdAt); err != nil {
			return nil, mapErr(err)
		}
		l.ConnectedAt = toTime(connectedAt)
		l.ClosedAt = toTimePtr(closedAt)
		l.CreatedAt = toTime(createdAt)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func rowsAffected(res sql.Result) int64 {
	n, err := res.RowsAffected()
	if err != nil {
		return 0
	}
	return n
}
