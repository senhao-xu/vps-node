package repo

import (
	"context"
	"database/sql"
	"time"
)

type Server struct {
	ID            int64
	Name          string
	Status        string
	CPUPercent    float64
	MemoryPercent float64
	DiskPercent   float64
	UptimeSeconds int64
	AgentVersion  string
	LastSeenAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const (
	ServerStatusActive   = "active"
	ServerStatusDisabled = "disabled"
	ServerStatusOffline  = "offline"
)

const serverSelect = `SELECT id, name, status, cpu_percent, memory_percent, disk_percent, uptime_seconds,
		     agent_version, last_seen_at, created_at, updated_at
		     FROM servers`

func getServerExec(ctx context.Context, q execer, id int64) (Server, error) {
	row := q.QueryRowContext(ctx, serverSelect+` WHERE id = ?`, id)
	return scanServer(row.Scan)
}

func (r *Repo) CreateServer(ctx context.Context, name, status string) (int64, error) {
	if status == "" {
		status = ServerStatusActive
	}
	now := nowUnix()
	res, err := r.DB.ExecContext(ctx,
		`INSERT INTO servers (name, status, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		name, status, now, now)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.LastInsertId()
}

func (r *Repo) GetServer(ctx context.Context, id int64) (Server, error) {
	row := r.DB.QueryRowContext(ctx, serverSelect+` WHERE id = ?`, id)
	return scanServer(row.Scan)
}

func (r *Repo) ListServers(ctx context.Context) ([]Server, error) {
	rows, err := r.DB.QueryContext(ctx, serverSelect+` ORDER BY id`)
	if err != nil {
		return nil, mapErr(err)
	}
	return collectServers(rows)
}

func (r *Repo) ListServersPage(ctx context.Context, page, size int) ([]Server, int64, error) {
	var total int64
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM servers`).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	page, size = normalizePage(page, size)
	rows, err := r.DB.QueryContext(ctx, serverSelect+` ORDER BY id LIMIT ? OFFSET ?`, size, (page-1)*size)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	servers, err := collectServers(rows)
	if err != nil {
		return nil, 0, err
	}
	return servers, total, nil
}

func collectServers(rows *sql.Rows) ([]Server, error) {
	defer rows.Close()
	servers := []Server{}
	for rows.Next() {
		s, err := scanServer(rows.Scan)
		if err != nil {
			return nil, mapErr(err)
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

func (r *Repo) UpdateServer(ctx context.Context, id int64, name, status string) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE servers SET name = ?, status = ?, updated_at = ? WHERE id = ?`,
		name, status, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) SetServerStatus(ctx context.Context, id int64, status string) error {
	_, err := r.DB.ExecContext(ctx, `UPDATE servers SET status = ?, updated_at = ? WHERE id = ?`, status, nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) UpdateServerMetrics(ctx context.Context, id int64, cpu, memory, disk float64, uptimeSeconds int64, seenAt time.Time) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE servers SET cpu_percent = ?, memory_percent = ?, disk_percent = ?, uptime_seconds = ?, last_seen_at = ?, updated_at = ?
		 WHERE id = ?`,
		cpu, memory, disk, uptimeSeconds, seenAt.Unix(), nowUnix(), id)
	return mapErr(err)
}

func (r *Repo) DeleteServer(ctx context.Context, id int64) error {
	_, err := r.DB.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
	return mapErr(err)
}

func scanServer(scan func(dest ...any) error) (Server, error) {
	var s Server
	var lastSeenAt sql.NullInt64
	var createdAt, updatedAt int64
	err := scan(&s.ID, &s.Name, &s.Status, &s.CPUPercent, &s.MemoryPercent, &s.DiskPercent,
		&s.UptimeSeconds, &s.AgentVersion, &lastSeenAt, &createdAt, &updatedAt)
	if err != nil {
		return Server{}, mapErr(err)
	}
	s.LastSeenAt = toTimePtr(lastSeenAt)
	s.CreatedAt = toTime(createdAt)
	s.UpdatedAt = toTime(updatedAt)
	return s, nil
}
