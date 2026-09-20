package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

const busyTimeoutMS = 5000

type DB struct {
	*sql.DB
}

func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", buildDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if path == ":memory:" {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", path, err)
	}
	return &DB{DB: sqlDB}, nil
}

func buildDSN(path string) string {
	return fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path, busyTimeoutMS)
}

func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
