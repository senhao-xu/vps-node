package repo

import (
	"context"
	"database/sql"
	"time"
)

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
