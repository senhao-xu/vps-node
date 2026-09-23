package repo

import (
	"context"
	"database/sql"
	"time"
)

type UserPatch struct {
	SetUsername       bool
	Username          string
	SetStatus         bool
	Status            string
	SetTransferEnable bool
	TransferEnable    int64
	SetSpeedLimit     bool
	SpeedLimit        int64
	SetDeviceLimit    bool
	DeviceLimit       int64
	SetStartedAt      bool
	StartedAt         *time.Time
	SetExpiresAt      bool
	ExpiresAt         *time.Time
}

func bumpRevisionExec(ctx context.Context, q execer, serverID int64) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO server_revisions (server_id, revision, updated_at) VALUES (?, 1, ?)
		 ON CONFLICT (server_id) DO UPDATE SET revision = revision + 1, updated_at = excluded.updated_at`,
		serverID, nowUnix())
	return mapErr(err)
}

func listServerIDsByUserExec(ctx context.Context, q execer, userID int64) ([]int64, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT DISTINCT n.server_id FROM user_nodes un JOIN nodes n ON n.id = un.node_id WHERE un.user_id = ?`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapErr(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func listServerIDsByNodesExec(ctx context.Context, q execer, nodeIDs []int64) ([]int64, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	rows, err := q.QueryContext(ctx,
		`SELECT DISTINCT server_id FROM nodes WHERE id IN (`+inPlaceholders(len(nodeIDs))+`)`,
		int64Args(nodeIDs)...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, mapErr(err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func unionInt64(a, b []int64) []int64 {
	seen := make(map[int64]bool, len(a)+len(b))
	out := make([]int64, 0, len(a)+len(b))
	for _, v := range append(append([]int64{}, a...), b...) {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func (r *Repo) CreateUserWithNodes(ctx context.Context, n NewUser, nodeIDs []int64) (int64, error) {
	unique := dedupeInt64(nodeIDs)
	var id int64
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		var err error
		id, err = insertUserExec(ctx, tx, n)
		if err != nil {
			return err
		}
		now := nowUnix()
		for _, nodeID := range unique {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (?, ?, ?)`, id, nodeID, now); err != nil {
				return mapErr(err)
			}
		}
		serverIDs, err := listServerIDsByNodesExec(ctx, tx, unique)
		if err != nil {
			return err
		}
		for _, serverID := range serverIDs {
			if err := bumpRevisionExec(ctx, tx, serverID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repo) UpdateUserAndBump(ctx context.Context, userID int64, p UserPatch) (User, error) {
	var u User
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		existing, err := getUserExec(ctx, tx, userID)
		if err != nil {
			return err
		}

		status := existing.Status
		if p.SetStatus {
			status = p.Status
		}
		if p.SetExpiresAt && p.ExpiresAt != nil && p.ExpiresAt.Before(time.Now()) {
			status = UserStatusExpired
		}
		quota := existing.TransferEnable
		if p.SetTransferEnable {
			quota = p.TransferEnable
		}
		speedLimit := existing.SpeedLimit
		if p.SetSpeedLimit {
			speedLimit = p.SpeedLimit
		}
		deviceLimit := existing.DeviceLimit
		if p.SetDeviceLimit {
			deviceLimit = p.DeviceLimit
		}
		startedAt := existing.StartedAt
		if p.SetStartedAt {
			startedAt = p.StartedAt
		}
		expiresAt := existing.ExpiresAt
		if p.SetExpiresAt {
			expiresAt = p.ExpiresAt
		}
		username := existing.Username
		if p.SetUsername {
			username = p.Username
		}

		res, err := tx.ExecContext(ctx,
			`UPDATE users SET username = ?, status = ?, transfer_enable = ?, speed_limit = ?, device_limit = ?, started_at = ?, expires_at = ?, updated_at = ? WHERE id = ?`,
			username, status, quota, speedLimit, deviceLimit, timeArg(startedAt), timeArg(expiresAt), nowUnix(), userID)
		if err != nil {
			return mapErr(err)
		}
		if rowsAffected(res) == 0 {
			return ErrNotFound
		}

		affectsAgentConfig := p.SetStatus || p.SetTransferEnable || p.SetStartedAt || p.SetExpiresAt ||
			p.SetDeviceLimit || p.SetSpeedLimit
		if !affectsAgentConfig {
			u, err = getUserExec(ctx, tx, userID)
			return err
		}

		serverIDs, err := listServerIDsByUserExec(ctx, tx, userID)
		if err != nil {
			return err
		}
		for _, serverID := range serverIDs {
			if err := bumpRevisionExec(ctx, tx, serverID); err != nil {
				return err
			}
		}
		u, err = getUserExec(ctx, tx, userID)
		return err
	})
	if err != nil {
		return User{}, err
	}
	return u, nil
}

func (r *Repo) DeleteUserAndBump(ctx context.Context, userID int64) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		serverIDs, err := listServerIDsByUserExec(ctx, tx, userID)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, userID)
		if err != nil {
			return mapErr(err)
		}
		if rowsAffected(res) == 0 {
			return ErrNotFound
		}
		for _, serverID := range serverIDs {
			if err := bumpRevisionExec(ctx, tx, serverID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repo) ResetUserTrafficAndBump(ctx context.Context, userID int64) (User, error) {
	var u User
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE users SET u = 0, d = 0, updated_at = ? WHERE id = ?`, nowUnix(), userID)
		if err != nil {
			return mapErr(err)
		}
		if rowsAffected(res) == 0 {
			return ErrNotFound
		}
		serverIDs, err := listServerIDsByUserExec(ctx, tx, userID)
		if err != nil {
			return err
		}
		for _, serverID := range serverIDs {
			if err := bumpRevisionExec(ctx, tx, serverID); err != nil {
				return err
			}
		}
		u, err = getUserExec(ctx, tx, userID)
		return err
	})
	if err != nil {
		return User{}, err
	}
	return u, nil
}

func (r *Repo) SetUserNodesAndBump(ctx context.Context, userID int64, nodeIDs []int64) ([]int64, error) {
	unique := dedupeInt64(nodeIDs)
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		if _, err := getUserExec(ctx, tx, userID); err != nil {
			return err
		}
		oldServers, err := listServerIDsByUserExec(ctx, tx, userID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM user_nodes WHERE user_id = ?`, userID); err != nil {
			return mapErr(err)
		}
		now := nowUnix()
		for _, nodeID := range unique {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (?, ?, ?)`, userID, nodeID, now); err != nil {
				return mapErr(err)
			}
		}
		newServers, err := listServerIDsByNodesExec(ctx, tx, unique)
		if err != nil {
			return err
		}
		for _, serverID := range unionInt64(oldServers, newServers) {
			if err := bumpRevisionExec(ctx, tx, serverID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return unique, nil
}

func (r *Repo) CreateNodeAndBump(ctx context.Context, n NewNode) (int64, error) {
	var id int64
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		var err error
		id, err = insertNodeExec(ctx, tx, n)
		if err != nil {
			return err
		}
		return bumpRevisionExec(ctx, tx, n.ServerID)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repo) UpdateNodeAndBump(ctx context.Context, nodeID, serverID int64, address, name string, port int, settings string, secretEnc []byte, rate float64, tags string, status *string) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		if status != nil {
			res, err := tx.ExecContext(ctx,
				`UPDATE nodes SET address = ?, name = ?, port = ?, protocol_settings = ?, secret_enc = ?, rate = ?, tags = ?, status = ?, updated_at = ? WHERE id = ?`,
				address, name, port, settings, secretEnc, rate, tags, *status, nowUnix(), nodeID)
			if err != nil {
				return mapErr(err)
			}
			if rowsAffected(res) == 0 {
				return ErrNotFound
			}
		} else {
			res, err := tx.ExecContext(ctx,
				`UPDATE nodes SET address = ?, name = ?, port = ?, protocol_settings = ?, secret_enc = ?, rate = ?, tags = ?, updated_at = ? WHERE id = ?`,
				address, name, port, settings, secretEnc, rate, tags, nowUnix(), nodeID)
			if err != nil {
				return mapErr(err)
			}
			if rowsAffected(res) == 0 {
				return ErrNotFound
			}
		}
		return bumpRevisionExec(ctx, tx, serverID)
	})
}

func (r *Repo) DeleteNodeAndBump(ctx context.Context, nodeID, serverID int64) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, nodeID)
		if err != nil {
			return mapErr(err)
		}
		if rowsAffected(res) == 0 {
			return ErrNotFound
		}
		return bumpRevisionExec(ctx, tx, serverID)
	})
}

func (r *Repo) UpdateServerAndBump(ctx context.Context, id int64, name, status string) (Server, error) {
	var s Server
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE servers SET name = ?, status = ?, updated_at = ? WHERE id = ?`,
			name, status, nowUnix(), id)
		if err != nil {
			return mapErr(err)
		}
		if rowsAffected(res) == 0 {
			return ErrNotFound
		}
		if err := bumpRevisionExec(ctx, tx, id); err != nil {
			return err
		}
		s, err = getServerExec(ctx, tx, id)
		return err
	})
	if err != nil {
		return Server{}, err
	}
	return s, nil
}

func (r *Repo) DeleteServerCascade(ctx context.Context, id int64) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
		if err != nil {
			return mapErr(err)
		}
		if rowsAffected(res) == 0 {
			return ErrNotFound
		}
		return nil
	})
}
