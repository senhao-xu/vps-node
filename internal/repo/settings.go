package repo

import (
	"context"
	"database/sql"
)

func (r *Repo) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.DB.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", mapErr(err)
	}
	return value, nil
}

func (r *Repo) SetSetting(ctx context.Context, key, value string) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, nowUnix())
	return mapErr(err)
}

func (r *Repo) SetSettings(ctx context.Context, values map[string]string) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		for key, value := range values {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
				 ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
				key, value, nowUnix()); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func (r *Repo) ListSettings(ctx context.Context) (map[string]string, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, mapErr(err)
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

func (r *Repo) GetSettingOr(ctx context.Context, key, fallback string) (string, error) {
	value, err := r.GetSetting(ctx, key)
	if err != nil {
		if err == ErrNotFound {
			return fallback, nil
		}
		return "", err
	}
	return value, nil
}
