package repo

import (
	"context"
	"time"
)

type Admin struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (r *Repo) CreateAdmin(ctx context.Context, username, passwordHash string) (int64, error) {
	now := nowUnix()
	res, err := r.DB.ExecContext(ctx,
		`INSERT INTO admins (username, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		username, passwordHash, now, now)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

func (r *Repo) GetAdmin(ctx context.Context, id int64) (Admin, error) {
	var a Admin
	var createdAt, updatedAt int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at, updated_at FROM admins WHERE id = ?`, id).
		Scan(&a.ID, &a.Username, &a.PasswordHash, &createdAt, &updatedAt)
	if err != nil {
		return Admin{}, mapErr(err)
	}
	a.CreatedAt = toTime(createdAt)
	a.UpdatedAt = toTime(updatedAt)
	return a, nil
}

func (r *Repo) GetAdminByUsername(ctx context.Context, username string) (Admin, error) {
	var a Admin
	var createdAt, updatedAt int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, username, password_hash, created_at, updated_at FROM admins WHERE username = ?`, username).
		Scan(&a.ID, &a.Username, &a.PasswordHash, &createdAt, &updatedAt)
	if err != nil {
		return Admin{}, mapErr(err)
	}
	a.CreatedAt = toTime(createdAt)
	a.UpdatedAt = toTime(updatedAt)
	return a, nil
}

func (r *Repo) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, mapErr(err)
}

func (r *Repo) UpdateAdminPassword(ctx context.Context, id int64, passwordHash string) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE admins SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash, nowUnix(), id)
	return mapErr(err)
}
