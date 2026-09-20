package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (r *Repo) RegisterAgent(ctx context.Context, serverID int64, tokenHash, version string) (int64, error) {
	var agentID int64
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, `SELECT id FROM agents WHERE server_id = ?`, serverID)
		err := row.Scan(&agentID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			now := nowUnix()
			res, insertErr := tx.ExecContext(ctx,
				`INSERT INTO agents (server_id, token_hash, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
				serverID, tokenHash, version, now, now)
			if insertErr != nil {
				return mapErr(insertErr)
			}
			agentID, insertErr = res.LastInsertId()
			if insertErr != nil {
				return mapErr(insertErr)
			}
		case err != nil:
			return mapErr(err)
		default:
			if _, err := tx.ExecContext(ctx,
				`UPDATE agents SET token_hash = ?, version = ?, updated_at = ? WHERE id = ?`,
				tokenHash, version, nowUnix(), agentID); err != nil {
				return mapErr(err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE servers SET register_token_hash = NULL, register_token_expires_at = NULL, updated_at = ? WHERE id = ?`,
			nowUnix(), serverID); err != nil {
			return mapErr(err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return agentID, nil
}

func (r *Repo) RecordHeartbeat(ctx context.Context, agentID, serverID int64, version string, cpu, memory, disk float64, uptimeSeconds int64, seenAt time.Time) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		now := nowUnix()
		if _, err := tx.ExecContext(ctx,
			`UPDATE agents SET version = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
			version, seenAt.Unix(), now, agentID); err != nil {
			return mapErr(err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE servers SET cpu_percent = ?, memory_percent = ?, disk_percent = ?, uptime_seconds = ?, agent_version = ?, last_seen_at = ?, updated_at = ?
			 WHERE id = ?`,
			cpu, memory, disk, uptimeSeconds, version, seenAt.Unix(), now, serverID); err != nil {
			return mapErr(err)
		}
		return nil
	})
}
