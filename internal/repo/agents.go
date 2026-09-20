package repo

import (
	"context"
	"database/sql"
	"time"
)

type Agent struct {
	ID         int64
	ServerID   int64
	TokenHash  string
	Version    string
	LastSeenAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (r *Repo) CreateAgent(ctx context.Context, serverID int64, tokenHash, version string) (int64, error) {
	now := nowUnix()
	res, err := r.DB.ExecContext(ctx,
		`INSERT INTO agents (server_id, token_hash, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		serverID, tokenHash, version, now, now)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

func (r *Repo) GetAgent(ctx context.Context, id int64) (Agent, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT id, server_id, token_hash, version, last_seen_at, created_at, updated_at FROM agents WHERE id = ?`, id)
	return scanAgent(row.Scan)
}

func (r *Repo) GetAgentByTokenHash(ctx context.Context, tokenHash string) (Agent, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT id, server_id, token_hash, version, last_seen_at, created_at, updated_at FROM agents WHERE token_hash = ?`, tokenHash)
	return scanAgent(row.Scan)
}

func (r *Repo) GetAgentByServerID(ctx context.Context, serverID int64) (Agent, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT id, server_id, token_hash, version, last_seen_at, created_at, updated_at FROM agents WHERE server_id = ?`, serverID)
	return scanAgent(row.Scan)
}

func (r *Repo) RotateAgentTokenHash(ctx context.Context, id int64, tokenHash string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE agents SET token_hash = ?, updated_at = ? WHERE id = ?`, tokenHash, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) UpdateAgentHeartbeat(ctx context.Context, id int64, version string, seenAt time.Time) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE agents SET version = ?, last_seen_at = ?, updated_at = ? WHERE id = ?`,
		version, seenAt.Unix(), nowUnix(), id)
	return mapErr(err)
}

func scanAgent(scan func(dest ...any) error) (Agent, error) {
	var a Agent
	var lastSeenAt sql.NullInt64
	var createdAt, updatedAt int64
	err := scan(&a.ID, &a.ServerID, &a.TokenHash, &a.Version, &lastSeenAt, &createdAt, &updatedAt)
	if err != nil {
		return Agent{}, mapErr(err)
	}
	a.LastSeenAt = toTimePtr(lastSeenAt)
	a.CreatedAt = toTime(createdAt)
	a.UpdatedAt = toTime(updatedAt)
	return a, nil
}
