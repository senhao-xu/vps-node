package repo

import (
	"context"
	"database/sql"
	"time"
)

type UserSubscription struct {
	UserID    int64
	TokenHash string
	TokenEnc  []byte
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SubscriptionNode struct {
	Node
	ServerAddress string
}

func insertSubscriptionExec(ctx context.Context, q execer, userID int64, tokenHash string, tokenEnc []byte) error {
	now := nowUnix()
	_, err := q.ExecContext(ctx, `INSERT INTO user_subscriptions (user_id, token_hash, token_enc, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, userID, tokenHash, tokenEnc, now, now)
	return mapErr(err)
}

func (r *Repo) CreateUserWithNodesAndSubscription(ctx context.Context, n NewUser, nodeIDs []int64, tokenHash string, tokenEnc []byte) (int64, error) {
	unique := dedupeInt64(nodeIDs)
	var id int64
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		var err error
		id, err = insertUserExec(ctx, tx, n)
		if err != nil {
			return err
		}
		if err := insertSubscriptionExec(ctx, tx, id, tokenHash, tokenEnc); err != nil {
			return err
		}
		now := nowUnix()
		for _, nodeID := range unique {
			if _, err := tx.ExecContext(ctx, `INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (?, ?, ?)`, id, nodeID, now); err != nil {
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
	return id, err
}

func (r *Repo) GetUserSubscription(ctx context.Context, userID int64) (UserSubscription, error) {
	return scanSubscription(r.DB.QueryRowContext(ctx, `SELECT user_id, token_hash, token_enc, created_at, updated_at FROM user_subscriptions WHERE user_id = ?`, userID).Scan)
}

func (r *Repo) CreateUserSubscription(ctx context.Context, userID int64, tokenHash string, tokenEnc []byte) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		if _, err := getUserExec(ctx, tx, userID); err != nil {
			return err
		}
		return insertSubscriptionExec(ctx, tx, userID, tokenHash, tokenEnc)
	})
}

func (r *Repo) RotateUserSubscription(ctx context.Context, userID int64, tokenHash string, tokenEnc []byte) error {
	res, err := r.DB.ExecContext(ctx, `UPDATE user_subscriptions SET token_hash = ?, token_enc = ?, updated_at = ? WHERE user_id = ?`, tokenHash, tokenEnc, nowUnix(), userID)
	if err != nil {
		return mapErr(err)
	}
	if rowsAffected(res) == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) ResolveSubscription(ctx context.Context, tokenHash string) (User, error) {
	row := r.DB.QueryRowContext(ctx, `SELECT users.id, users.uuid, users.username, users.token_hash, users.status, users.transfer_enable, users.u, users.d, users.speed_limit, users.device_limit, users.online_count, users.last_online_at, users.started_at, users.expires_at, users.created_at, users.updated_at
		FROM users JOIN user_subscriptions us ON us.user_id = users.id WHERE us.token_hash = ?`, tokenHash)
	return scanUser(row.Scan)
}

func (r *Repo) ListSubscriptionNodes(ctx context.Context, userID int64) ([]SubscriptionNode, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT n.id, n.server_id, n.name, n.protocol, n.port, n.protocol_settings, n.secret_enc, n.status, n.created_at, n.updated_at, s.address
		FROM nodes n JOIN user_nodes un ON un.node_id = n.id JOIN servers s ON s.id = n.server_id
		WHERE un.user_id = ? AND n.status = 'active' AND s.status = 'active' ORDER BY n.id`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := []SubscriptionNode{}
	for rows.Next() {
		var n SubscriptionNode
		var created, updated int64
		if err := rows.Scan(&n.ID, &n.ServerID, &n.Name, &n.Protocol, &n.Port, &n.ProtocolSettings, &n.SecretEnc, &n.Status, &created, &updated, &n.ServerAddress); err != nil {
			return nil, mapErr(err)
		}
		n.CreatedAt, n.UpdatedAt = toTime(created), toTime(updated)
		out = append(out, n)
	}
	return out, rows.Err()
}

func scanSubscription(scan func(...any) error) (UserSubscription, error) {
	var s UserSubscription
	var created, updated int64
	if err := scan(&s.UserID, &s.TokenHash, &s.TokenEnc, &created, &updated); err != nil {
		return UserSubscription{}, mapErr(err)
	}
	s.CreatedAt, s.UpdatedAt = toTime(created), toTime(updated)
	return s, nil
}
