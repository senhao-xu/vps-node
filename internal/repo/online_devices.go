package repo

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type OnlineDevice struct {
	ID         int64
	UserID     int64
	NodeID     int64
	ServerID   int64
	IP         string
	Online     int
	LastSeenAt time.Time
	CreatedAt  time.Time
}

type NewOnlineDevice struct {
	UserID int64
	NodeID int64
	IP     string
	Online int
}

func (r *Repo) ListDevicesByUser(ctx context.Context, userID int64) ([]OnlineDevice, error) {
	rows, err := r.DB.QueryContext(ctx, onlineDeviceSelect+` WHERE user_id = ? ORDER BY last_seen_at DESC, id`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectOnlineDevices(rows)
}

func (r *Repo) ListDevicesByNode(ctx context.Context, nodeID int64) ([]OnlineDevice, error) {
	rows, err := r.DB.QueryContext(ctx, onlineDeviceSelect+` WHERE node_id = ? ORDER BY last_seen_at DESC, id`, nodeID)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectOnlineDevices(rows)
}

func (r *Repo) DeleteStaleDevicesBatch(ctx context.Context, cutoff time.Time, limit int64) (int64, error) {
	var deleted int64
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx,
			`SELECT id, user_id FROM online_devices WHERE last_seen_at < ? LIMIT ?`, cutoff.Unix(), limit)
		if err != nil {
			return mapErr(err)
		}
		ids := []any{}
		affected := map[int64]bool{}
		for rows.Next() {
			var id, userID int64
			if err := rows.Scan(&id, &userID); err != nil {
				rows.Close()
				return mapErr(err)
			}
			ids = append(ids, id)
			affected[userID] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return mapErr(err)
		}
		if len(ids) == 0 {
			return nil
		}
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
		res, err := tx.ExecContext(ctx, `DELETE FROM online_devices WHERE id IN (`+placeholders+`)`, ids...)
		if err != nil {
			return mapErr(err)
		}
		deleted = rowsAffected(res)
		for userID := range affected {
			if err := refreshOnlineCountExec(ctx, tx, userID); err != nil {
				return err
			}
		}
		return nil
	})
	return deleted, err
}

func refreshOnlineCountExec(ctx context.Context, q execer, userID int64) error {
	_, err := q.ExecContext(ctx,
		`UPDATE users SET online_count = (SELECT COUNT(DISTINCT od.ip) FROM online_devices od WHERE od.user_id = users.id), updated_at = ? WHERE id = ?`,
		nowUnix(), userID)
	return mapErr(err)
}

const onlineDeviceSelect = `SELECT id, user_id, node_id, server_id, ip, online, last_seen_at, created_at FROM online_devices`

func collectOnlineDevices(rows *sql.Rows) ([]OnlineDevice, error) {
	defer rows.Close()
	devices := []OnlineDevice{}
	for rows.Next() {
		var d OnlineDevice
		var lastSeenAt, createdAt int64
		if err := rows.Scan(&d.ID, &d.UserID, &d.NodeID, &d.ServerID, &d.IP, &d.Online, &lastSeenAt, &createdAt); err != nil {
			return nil, mapErr(err)
		}
		d.LastSeenAt = toTime(lastSeenAt)
		d.CreatedAt = toTime(createdAt)
		devices = append(devices, d)
	}
	return devices, rows.Err()
}
