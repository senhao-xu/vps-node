package repo

import (
	"context"
	"database/sql"
	"errors"
)

type UserNodePair struct {
	UserID int64
	NodeID int64
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

func (r *Repo) IngestTrafficBatch(ctx context.Context, agentID, seq int64, records []NewTrafficRecord, userDeltas map[int64]int64) (int64, bool, error) {
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
		for _, rec := range records {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO traffic_records (user_id, node_id, server_id, upload_bytes, download_bytes, created_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				rec.UserID, rec.NodeID, rec.ServerID, rec.UploadBytes, rec.DownloadBytes, rec.CreatedAt.Unix()); err != nil {
				return mapErr(err)
			}
		}
		for userID, delta := range userDeltas {
			if _, err := tx.ExecContext(ctx,
				`UPDATE users SET used_bytes = used_bytes + ?, updated_at = ? WHERE id = ?`,
				delta, nowUnix(), userID); err != nil {
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

func (r *Repo) IngestLogBatch(ctx context.Context, agentID, seq int64, logs []NewConnectionLog) (int64, bool, error) {
	var (
		count     int64
		duplicate bool
	)
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			`SELECT logs FROM connection_log_batches WHERE agent_id = ? AND seq = ?`, agentID, seq).Scan(&count)
		if err == nil {
			duplicate = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return mapErr(err)
		}
		now := nowUnix()
		for _, l := range logs {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO connection_logs (user_id, node_id, server_id, ip, protocol, upload_bytes, download_bytes, connected_at, closed_at, status, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				l.UserID, l.NodeID, l.ServerID, l.IP, l.Protocol, l.UploadBytes, l.DownloadBytes,
				l.ConnectedAt.Unix(), timeArg(l.ClosedAt), l.Status, now); err != nil {
				return mapErr(err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO connection_log_batches (agent_id, seq, received_at, logs) VALUES (?, ?, ?, ?)`,
			agentID, seq, now, len(logs)); err != nil {
			return mapErr(err)
		}
		count = int64(len(logs))
		return nil
	})
	return count, duplicate, err
}

func (r *Repo) LogBatchCount(ctx context.Context, agentID, seq int64) (int64, bool, error) {
	var count int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT logs FROM connection_log_batches WHERE agent_id = ? AND seq = ?`, agentID, seq).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, mapErr(err)
	}
	return count, true, nil
}
