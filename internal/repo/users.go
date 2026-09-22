package repo

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type User struct {
	ID             int64
	UUID           string
	Username       string
	TokenHash      string
	Status         string
	TransferEnable int64
	U              int64
	D              int64
	SpeedLimit     int64
	DeviceLimit    int64
	OnlineCount    int64
	LastOnlineAt   *time.Time
	StartedAt      *time.Time
	ExpiresAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (u User) UsedBytes() int64 {
	return u.U + u.D
}

type NewUser struct {
	UUID           string
	Username       string
	TokenHash      string
	Status         string
	TransferEnable int64
	SpeedLimit     int64
	DeviceLimit    int64
	StartedAt      *time.Time
	ExpiresAt      *time.Time
}

type UserFilter struct {
	Query     string
	TokenHash string
	Status    string
	Expiry    string
	Page      int
	PageSize  int
}

const (
	UserStatusActive   = "active"
	UserStatusDisabled = "disabled"
	UserStatusExpired  = "expired"
)

const userSelect = `SELECT id, uuid, username, token_hash, status, transfer_enable, u, d, speed_limit, device_limit, online_count, last_online_at, started_at, expires_at, created_at, updated_at FROM users`

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func insertUserExec(ctx context.Context, q execer, n NewUser) (int64, error) {
	if n.Status == "" {
		n.Status = UserStatusActive
	}
	now := nowUnix()
	res, err := q.ExecContext(ctx,
		`INSERT INTO users (uuid, username, token_hash, status, transfer_enable, u, d, speed_limit, device_limit, started_at, expires_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?, ?, ?)`,
		n.UUID, n.Username, n.TokenHash, n.Status, n.TransferEnable, n.SpeedLimit, n.DeviceLimit,
		timeArg(n.StartedAt), timeArg(n.ExpiresAt), now, now)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

func getUserExec(ctx context.Context, q execer, id int64) (User, error) {
	row := q.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id)
	return scanUser(row.Scan)
}

func (r *Repo) CreateUser(ctx context.Context, n NewUser) (int64, error) {
	return insertUserExec(ctx, r.DB, n)
}

func (r *Repo) GetUser(ctx context.Context, id int64) (User, error) {
	return getUserExec(ctx, r.DB, id)
}

func (r *Repo) GetUserByUUID(ctx context.Context, uuid string) (User, error) {
	row := r.DB.QueryRowContext(ctx, userSelect+` WHERE uuid = ?`, uuid)
	return scanUser(row.Scan)
}

func (r *Repo) GetUserByTokenHash(ctx context.Context, tokenHash string) (User, error) {
	row := r.DB.QueryRowContext(ctx, userSelect+` WHERE token_hash = ?`, tokenHash)
	return scanUser(row.Scan)
}

func (r *Repo) ListUsers(ctx context.Context, f UserFilter) ([]User, int64, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if f.Query != "" {
		tokenHash := f.TokenHash
		if tokenHash == "" {
			tokenHash = f.Query
		}
		where = append(where, "(uuid = ? OR token_hash = ? OR username = ?)")
		args = append(args, f.Query, tokenHash, f.Query)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	switch f.Expiry {
	case "valid":
		where = append(where, "(expires_at IS NULL OR expires_at > ?)")
		args = append(args, nowUnix())
	case "expired":
		where = append(where, "expires_at IS NOT NULL AND expires_at <= ?")
		args = append(args, nowUnix())
	}
	whereSQL := strings.Join(where, " AND ")

	var total int64
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}

	page, size := normalizePage(f.Page, f.PageSize)
	rows, err := r.DB.QueryContext(ctx,
		userSelect+` WHERE `+whereSQL+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, 0, mapErr(err)
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *Repo) ListUsersAll(ctx context.Context) ([]User, error) {
	rows, err := r.DB.QueryContext(ctx, userSelect+` ORDER BY id ASC`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, mapErr(err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repo) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, mapErr(err)
}

func (r *Repo) ListEligibleUsersByServer(ctx context.Context, serverID int64, now time.Time) ([]User, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT u.id, u.uuid, u.username, u.token_hash, u.status, u.transfer_enable, u.u, u.d, u.speed_limit, u.device_limit, u.online_count, u.last_online_at, u.started_at, u.expires_at, u.created_at, u.updated_at
		 FROM users u
		 JOIN user_nodes un ON un.user_id = u.id
		 JOIN nodes n ON n.id = un.node_id
		 WHERE n.server_id = ?
		   AND u.status = 'active'
		   AND (u.expires_at IS NULL OR u.expires_at > ?)
		   AND (u.transfer_enable = 0 OR (u.u + u.d) < u.transfer_enable)
		 GROUP BY u.id ORDER BY u.id`, serverID, now.Unix())
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, mapErr(err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repo) SetUserStatus(ctx context.Context, id int64, status string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE users SET status = ?, updated_at = ? WHERE id = ?`, status, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) SetUserTransferEnable(ctx context.Context, id int64, transferEnable int64) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE users SET transfer_enable = ?, updated_at = ? WHERE id = ?`, transferEnable, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) SetUserExpiry(ctx context.Context, id int64, startedAt, expiresAt *time.Time) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE users SET started_at = ?, expires_at = ?, updated_at = ? WHERE id = ?`,
		timeArg(startedAt), timeArg(expiresAt), nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) ResetUserTokenHash(ctx context.Context, id int64, tokenHash string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE users SET token_hash = ?, updated_at = ? WHERE id = ?`, tokenHash, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) ResetUserTraffic(ctx context.Context, id int64) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE users SET u = 0, d = 0, updated_at = ? WHERE id = ?`, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) AddUserUsedBytes(ctx context.Context, id int64, uploadBytes, downloadBytes int64) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE users SET u = u + ?, d = d + ?, updated_at = ? WHERE id = ?`,
		uploadBytes, downloadBytes, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) DeleteUser(ctx context.Context, id int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return mapErr(err)
}

func scanUser(scan func(dest ...any) error) (User, error) {
	var u User
	var lastOnlineAt, startedAt, expiresAt sql.NullInt64
	var createdAt, updatedAt int64
	err := scan(&u.ID, &u.UUID, &u.Username, &u.TokenHash, &u.Status, &u.TransferEnable, &u.U, &u.D,
		&u.SpeedLimit, &u.DeviceLimit, &u.OnlineCount, &lastOnlineAt,
		&startedAt, &expiresAt, &createdAt, &updatedAt)
	if err != nil {
		return User{}, mapErr(err)
	}
	u.LastOnlineAt = toTimePtr(lastOnlineAt)
	u.StartedAt = toTimePtr(startedAt)
	u.ExpiresAt = toTimePtr(expiresAt)
	u.CreatedAt = toTime(createdAt)
	u.UpdatedAt = toTime(updatedAt)
	return u, nil
}
