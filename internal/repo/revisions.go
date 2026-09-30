package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (r *Repo) GetServerRevision(ctx context.Context, serverID int64) (int64, error) {
	var revision int64
	err := r.DB.QueryRowContext(ctx, `SELECT revision FROM server_revisions WHERE server_id = ?`, serverID).Scan(&revision)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, mapErr(err)
	}
	return revision, nil
}

func (r *Repo) BumpServerRevision(ctx context.Context, serverID int64) (int64, error) {
	if err := bumpRevisionExec(ctx, r.DB, serverID); err != nil {
		return 0, err
	}
	return r.GetServerRevision(ctx, serverID)
}

func (r *Repo) ReconcileServerExpiry(ctx context.Context, serverID int64, now time.Time) (int64, error) {
	var revision int64
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO server_revisions (server_id, revision, updated_at)
			 VALUES (?, 0, ?) ON CONFLICT (server_id) DO NOTHING`, serverID, now.Unix()); err != nil {
			return mapErr(err)
		}
		var checkedAt int64
		if err := tx.QueryRowContext(ctx, `SELECT revision, expiry_checked_at FROM server_revisions WHERE server_id = ?`, serverID).Scan(&revision, &checkedAt); err != nil {
			return mapErr(err)
		}
		if now.Unix() <= checkedAt {
			return nil
		}
		var expired bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM users u JOIN user_nodes un ON un.user_id = u.id JOIN nodes n ON n.id = un.node_id
			WHERE n.server_id = ? AND u.status = 'active'
			  AND u.expires_at > ? AND u.expires_at <= ?
		)`, serverID, checkedAt, now.Unix()).Scan(&expired); err != nil {
			return mapErr(err)
		}
		if expired {
			if err := bumpRevisionExec(ctx, tx, serverID); err != nil {
				return err
			}
			revision++
		}
		_, err := tx.ExecContext(ctx, `UPDATE server_revisions SET expiry_checked_at = ? WHERE server_id = ?`, now.Unix(), serverID)
		return mapErr(err)
	})
	return revision, err
}
