package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func (d *DB) Migrate(ctx context.Context) error {
	_, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	applied, err := d.appliedVersions(ctx)
	if err != nil {
		return err
	}

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
		if applied[version] {
			continue
		}
		if err := d.applyMigration(ctx, version, name); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}

func (d *DB) appliedVersions(ctx context.Context) (map[int64]bool, error) {
	rows, err := d.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func (d *DB) applyMigration(ctx context.Context, version int64, name string) error {
	content, err := migrationsFS.ReadFile("migrations/" + name)
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, strftime('%s','now'))`, version); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
}

func migrationVersion(name string) (int64, error) {
	prefix, _, _ := strings.Cut(name, "_")
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid migration filename %q", name)
	}
	return version, nil
}
