package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"vps-node/internal/db"
)

var (
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("record conflict")
)

type Repo struct {
	DB *sql.DB
}

func New(sqlDB *sql.DB) *Repo {
	return &Repo{DB: sqlDB}
}

func Tx(ctx context.Context, sqlDB *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func mapErr(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return ErrNotFound
	case db.IsUniqueViolation(err):
		return ErrConflict
	default:
		return err
	}
}

func nowUnix() int64 {
	return time.Now().Unix()
}

func toTime(v int64) time.Time {
	return time.Unix(v, 0).UTC()
}

func toTimePtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := toTime(v.Int64)
	return &t
}

func timeArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}

func normalizePage(page, size int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return page, size
}
