package repo

import (
	"context"
	"database/sql"
	"errors"
	"math"
)

type UserNodePair struct {
	UserID int64
	NodeID int64
}

type UserTrafficDelta struct {
	U int64
	D int64
}

func (r *Repo) ListUserNodePairsByServer(ctx context.Context, serverID int64) ([]UserNodePair, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT un.user_id, un.node_id FROM user_nodes un
		 JOIN nodes n ON n.id = un.node_id
		 WHERE n.server_id = ? ORDER BY un.user_id, un.node_id`, serverID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	pairs := []UserNodePair{}
	for rows.Next() {
		var p UserNodePair
		if err := rows.Scan(&p.UserID, &p.NodeID); err != nil {
			return nil, mapErr(err)
		}
		pairs = append(pairs, p)
	}
	return pairs, rows.Err()
}

func (r *Repo) IngestTrafficBatch(ctx context.Context, agentID, seq int64, records []NewTrafficRecord) (int64, bool, error) {
	var (
		count     int64
		duplicate bool
	)
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			`SELECT records FROM traffic_batches WHERE agent_id = ? AND seq = ?`, agentID, seq).Scan(&count)
		if err == nil {
			duplicate = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return mapErr(err)
		}
		rates := map[int64]float64{}
		userDeltas := map[int64]UserTrafficDelta{}
		for _, rec := range records {
			rate, ok := rates[rec.NodeID]
			if !ok {
				if err := tx.QueryRowContext(ctx, `SELECT rate FROM nodes WHERE id = ?`, rec.NodeID).Scan(&rate); err != nil {
					return mapErr(err)
				}
				rates[rec.NodeID] = rate
			}
			u := scaleBytes(rec.U, rate)
			d := scaleBytes(rec.D, rate)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO traffic_records (user_id, node_id, server_id, u, d, created_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				rec.UserID, rec.NodeID, rec.ServerID, u, d, rec.CreatedAt.Unix()); err != nil {
				return mapErr(err)
			}
			delta := userDeltas[rec.UserID]
			delta.U += u
			delta.D += d
			userDeltas[rec.UserID] = delta
		}
		for userID, delta := range userDeltas {
			if _, err := tx.ExecContext(ctx,
				`UPDATE users SET u = u + ?, d = d + ?, updated_at = ? WHERE id = ?`,
				delta.U, delta.D, nowUnix(), userID); err != nil {
				return mapErr(err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO traffic_batches (agent_id, seq, received_at, records) VALUES (?, ?, ?, ?)`,
			agentID, seq, nowUnix(), len(records)); err != nil {
			return mapErr(err)
		}
		count = int64(len(records))
		return nil
	})
	return count, duplicate, err
}

func scaleBytes(v int64, rate float64) int64 {
	if rate <= 0 || rate == 1 || v == 0 {
		return v
	}
	scaled := math.Round(float64(v) * rate)
	if scaled >= float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(scaled)
}

func (r *Repo) TrafficBatchCount(ctx context.Context, agentID, seq int64) (int64, bool, error) {
	var count int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT records FROM traffic_batches WHERE agent_id = ? AND seq = ?`, agentID, seq).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, mapErr(err)
	}
	return count, true, nil
}

func (r *Repo) IngestDeviceBatch(ctx context.Context, agentID, seq, serverID int64, devices []NewOnlineDevice, onlineByUser map[int64]int, seenAt int64) (int64, bool, error) {
	var (
		count     int64
		duplicate bool
	)
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			`SELECT devices FROM device_batches WHERE agent_id = ? AND seq = ?`, agentID, seq).Scan(&count)
		if err == nil {
			duplicate = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return mapErr(err)
		}

		affected := map[int64]bool{}
		rows, err := tx.QueryContext(ctx, `SELECT DISTINCT user_id FROM online_devices WHERE server_id = ?`, serverID)
		if err != nil {
			return mapErr(err)
		}
		for rows.Next() {
			var userID int64
			if err := rows.Scan(&userID); err != nil {
				rows.Close()
				return mapErr(err)
			}
			affected[userID] = true
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return mapErr(err)
		}
		for _, d := range devices {
			affected[d.UserID] = true
		}

		if _, err := tx.ExecContext(ctx,
			`DELETE FROM online_devices WHERE server_id = ? AND last_seen_at < ?`, serverID, seenAt); err != nil {
			return mapErr(err)
		}
		for _, d := range devices {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO online_devices (user_id, node_id, server_id, ip, online, last_seen_at, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT (user_id, node_id, ip) DO UPDATE SET last_seen_at = excluded.last_seen_at, online = excluded.online`,
				d.UserID, d.NodeID, serverID, d.IP, d.Online, seenAt, nowUnix())
			if err != nil {
				return mapErr(err)
			}
			count += rowsAffected(res)
		}
		for userID := range affected {
			if err := refreshOnlineCountExec(ctx, tx, userID); err != nil {
				return err
			}
		}
		for userID, online := range onlineByUser {
			if online <= 0 {
				continue
			}
			if _, err := tx.ExecContext(ctx, `UPDATE users SET last_online_at = ? WHERE id = ?`, seenAt, userID); err != nil {
				return mapErr(err)
			}
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO device_batches (agent_id, seq, received_at, devices) VALUES (?, ?, ?, ?)`,
			agentID, seq, nowUnix(), count); err != nil {
			return mapErr(err)
		}
		return nil
	})
	return count, duplicate, err
}

func (r *Repo) DeviceBatchCount(ctx context.Context, agentID, seq int64) (int64, bool, error) {
	var count int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT devices FROM device_batches WHERE agent_id = ? AND seq = ?`, agentID, seq).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, mapErr(err)
	}
	return count, true, nil
}
