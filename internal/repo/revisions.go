package repo

import (
	"context"
	"database/sql"
	"errors"
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
