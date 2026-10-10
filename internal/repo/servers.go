package repo

import (
	"context"
	"database/sql"
	"time"
)

type Server struct {
	ID                          int64
	Name                        string
	Notes                       string
	PublicVisible               bool
	OfflineNotify               bool
	IPv6                        string
	ObservedIP                  string
	ObservedIPv6                string
	TrafficAccounting           string
	TrafficResetDay             int
	TrafficCorrectionBytes      int64
	TrafficCorrectionCycleStart int64
	BillingCycle                string
	IP                          string
	Region                      string
	PriceCents                  int64
	PriceCurrency               string
	TrafficLimitBytes           int64
	TrafficUsedBytes            int64
	ExpiresAt                   *time.Time
	SortOrder                   int64
	Status                      string
	CPUPercent                  float64
	MemoryPercent               float64
	DiskPercent                 float64
	UptimeSeconds               int64
	AgentVersion                string
	LastSeenAt                  *time.Time
	CreatedAt                   time.Time
	UpdatedAt                   time.Time
}

const (
	ServerStatusActive   = "active"
	ServerStatusDisabled = "disabled"
	ServerStatusOffline  = "offline"
)

const serverSelect = `SELECT id, name, notes, public_visible, offline_notify, ipv6, observed_ip, observed_ipv6, traffic_accounting, traffic_reset_day, traffic_correction_bytes, traffic_correction_cycle_start, billing_cycle, ip, region, price_cents, price_currency, traffic_limit_bytes, traffic_used_bytes, expires_at, sort_order, status, cpu_percent, memory_percent, disk_percent, uptime_seconds,
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

func (r *Repo) CreateServerWithAgentKey(ctx context.Context, name, status, keyHash string, keyEnc []byte, inventory ServerInventory) (int64, error) {
	if status == "" {
		status = ServerStatusActive
	}
	now := nowUnix()
	var expiresAt any
	if inventory.ExpiresAt != nil {
		expiresAt = inventory.ExpiresAt.Unix()
	}
	var id int64
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO servers (name, status, notes, public_visible, offline_notify, ipv6, traffic_accounting, traffic_reset_day, billing_cycle, ip, region, price_cents, price_currency, traffic_limit_bytes, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			name, status, inventory.Notes, inventory.PublicVisible, inventory.OfflineNotify, inventory.IPv6, inventory.TrafficAccounting, inventory.TrafficResetDay, inventory.BillingCycle, inventory.IP, inventory.Region, inventory.PriceCents, inventory.PriceCurrency, inventory.TrafficLimitBytes, expiresAt, now, now)
		if err != nil {
			return mapErr(err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO agents (server_id, key_hash, key_enc, version, created_at, updated_at) VALUES (?, ?, ?, '', ?, ?)`,
			id, keyHash, keyEnc, now, now)
		return mapErr(err)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (r *Repo) GetServer(ctx context.Context, id int64) (Server, error) {
	row := r.DB.QueryRowContext(ctx, serverSelect+` WHERE id = ?`, id)
	return scanServer(row.Scan)
}

func (r *Repo) ListServers(ctx context.Context) ([]Server, error) {
	rows, err := r.DB.QueryContext(ctx, serverSelect+` ORDER BY sort_order, id`)
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
	rows, err := r.DB.QueryContext(ctx, serverSelect+` ORDER BY sort_order, id LIMIT ? OFFSET ?`, size, (page-1)*size)
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
	var lastSeenAt, expiresAt sql.NullInt64
	var createdAt, updatedAt int64
	var publicVisible, offlineNotify int
	err := scan(&s.ID, &s.Name, &s.Notes, &publicVisible, &offlineNotify, &s.IPv6, &s.ObservedIP, &s.ObservedIPv6, &s.TrafficAccounting, &s.TrafficResetDay, &s.TrafficCorrectionBytes, &s.TrafficCorrectionCycleStart, &s.BillingCycle, &s.IP, &s.Region, &s.PriceCents, &s.PriceCurrency, &s.TrafficLimitBytes, &s.TrafficUsedBytes, &expiresAt, &s.SortOrder, &s.Status, &s.CPUPercent, &s.MemoryPercent, &s.DiskPercent,
		&s.UptimeSeconds, &s.AgentVersion, &lastSeenAt, &createdAt, &updatedAt)
	if err != nil {
		return Server{}, mapErr(err)
	}
	s.PublicVisible = publicVisible != 0
	s.OfflineNotify = offlineNotify != 0
	s.LastSeenAt = toTimePtr(lastSeenAt)
	s.ExpiresAt = toTimePtr(expiresAt)
	s.CreatedAt = toTime(createdAt)
	s.UpdatedAt = toTime(updatedAt)
	return s, nil
}

// ServerInventory contains optional billing and display information for a server.
type ServerInventory struct {
	Notes             string
	PublicVisible     bool
	OfflineNotify     bool
	IPv6              string
	TrafficAccounting string
	TrafficResetDay   int
	BillingCycle      string
	IP                string
	Region            string
	PriceCents        int64
	PriceCurrency     string
	TrafficLimitBytes int64
	ExpiresAt         *time.Time
}

func (r *Repo) ReorderServers(ctx context.Context, ids []int64) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		for i, id := range ids {
			res, err := tx.ExecContext(ctx, `UPDATE servers SET sort_order = ?, updated_at = ? WHERE id = ?`, i+1, nowUnix(), id)
			if err != nil {
				return mapErr(err)
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return ErrNotFound
			}
		}
		return nil
	})
}
