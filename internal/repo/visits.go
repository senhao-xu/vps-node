package repo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type NewVisitRecord struct {
	UserID    int64
	NodeID    int64
	ServerID  int64
	DestHost  string
	DestPort  int
	Network   string
	ClientIP  string
	CreatedAt time.Time
}

type VisitRecord struct {
	ID         int64
	UserID     int64
	Username   string
	NodeID     int64
	NodeName   string
	ServerID   int64
	ServerName string
	DestHost   string
	DestPort   int
	Network    string
	ClientIP   string
	CreatedAt  time.Time
}

type VisitFilter struct {
	UserID   int64
	ServerID int64
	NodeID   int64
	Host     string
	ClientIP string
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

type TopHost struct {
	DestHost string
	Hits     int64
}

func visitDay(t time.Time) int64 {
	return time.Unix(t.Unix(), 0).UTC().Truncate(24 * time.Hour).Unix()
}

func (r *Repo) IngestVisitBatch(ctx context.Context, agentID, seq, serverID int64, records []NewVisitRecord) (int64, bool, error) {
	var (
		count     int64
		duplicate bool
	)
	err := Tx(ctx, r.DB, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx,
			`SELECT records FROM visit_batches WHERE agent_id = ? AND seq = ?`, agentID, seq).Scan(&count)
		if err == nil {
			duplicate = true
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return mapErr(err)
		}
		for _, rec := range records {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO visit_records (user_id, node_id, server_id, dest_host, dest_port, network, client_ip, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				rec.UserID, rec.NodeID, serverID, rec.DestHost, rec.DestPort, rec.Network, rec.ClientIP, rec.CreatedAt.Unix()); err != nil {
				return mapErr(err)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO visit_daily_domains (day, user_id, node_id, server_id, dest_host, hits)
				 VALUES (?, ?, ?, ?, ?, 1)
				 ON CONFLICT (day, user_id, node_id, dest_host) DO UPDATE SET hits = hits + 1`,
				visitDay(rec.CreatedAt), rec.UserID, rec.NodeID, serverID, rec.DestHost); err != nil {
				return mapErr(err)
			}
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO visit_batches (agent_id, seq, received_at, records) VALUES (?, ?, ?, ?)`,
			agentID, seq, nowUnix(), len(records)); err != nil {
			return mapErr(err)
		}
		count = int64(len(records))
		return nil
	})
	return count, duplicate, err
}

func (r *Repo) VisitBatchCount(ctx context.Context, agentID, seq int64) (int64, bool, error) {
	var count int64
	err := r.DB.QueryRowContext(ctx,
		`SELECT records FROM visit_batches WHERE agent_id = ? AND seq = ?`, agentID, seq).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, mapErr(err)
	}
	return count, true, nil
}

func visitWhere(f VisitFilter) (string, []any) {
	where := []string{"1 = 1"}
	args := []any{}
	if f.UserID != 0 {
		where = append(where, "vr.user_id = ?")
		args = append(args, f.UserID)
	}
	if f.ServerID != 0 {
		where = append(where, "vr.server_id = ?")
		args = append(args, f.ServerID)
	}
	if f.NodeID != 0 {
		where = append(where, "vr.node_id = ?")
		args = append(args, f.NodeID)
	}
	if f.Host != "" {
		where = append(where, "vr.dest_host LIKE ?")
		args = append(args, "%"+strings.ToLower(f.Host)+"%")
	}
	if f.ClientIP != "" {
		where = append(where, "vr.client_ip LIKE ?")
		args = append(args, "%"+strings.ToLower(f.ClientIP)+"%")
	}
	if f.From != nil {
		where = append(where, "vr.created_at >= ?")
		args = append(args, f.From.Unix())
	}
	if f.To != nil {
		where = append(where, "vr.created_at < ?")
		args = append(args, f.To.Unix())
	}
	return strings.Join(where, " AND "), args
}

func (r *Repo) ListVisits(ctx context.Context, f VisitFilter) ([]VisitRecord, int64, error) {
	page, size := normalizePage(f.Page, f.PageSize)
	whereSQL, args := visitWhere(f)

	var total int64
	if err := r.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM visit_records vr WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT vr.id, vr.user_id, u.username, vr.node_id, n.name, vr.server_id, s.name,
		        vr.dest_host, vr.dest_port, vr.network, vr.client_ip, vr.created_at
		 FROM visit_records vr
		 JOIN users u ON u.id = vr.user_id
		 JOIN nodes n ON n.id = vr.node_id
		 JOIN servers s ON s.id = vr.server_id
		 WHERE `+whereSQL+` ORDER BY vr.created_at DESC, vr.id DESC LIMIT ? OFFSET ?`,
		append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()

	visits := []VisitRecord{}
	for rows.Next() {
		var v VisitRecord
		var createdAt int64
		if err := rows.Scan(&v.ID, &v.UserID, &v.Username, &v.NodeID, &v.NodeName, &v.ServerID, &v.ServerName,
			&v.DestHost, &v.DestPort, &v.Network, &v.ClientIP, &createdAt); err != nil {
			return nil, 0, mapErr(err)
		}
		v.CreatedAt = toTime(createdAt)
		visits = append(visits, v)
	}
	return visits, total, rows.Err()
}

func (r *Repo) TopVisitHosts(ctx context.Context, f VisitFilter, sinceDay int64, limit int) ([]TopHost, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	where := []string{"day >= ?"}
	args := []any{sinceDay}
	if f.UserID != 0 {
		where = append(where, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.ServerID != 0 {
		where = append(where, "server_id = ?")
		args = append(args, f.ServerID)
	}
	if f.NodeID != 0 {
		where = append(where, "node_id = ?")
		args = append(args, f.NodeID)
	}
	if f.Host != "" {
		where = append(where, "dest_host LIKE ?")
		args = append(args, "%"+strings.ToLower(f.Host)+"%")
	}

	rows, err := r.DB.QueryContext(ctx,
		`SELECT dest_host, COALESCE(SUM(hits), 0) FROM visit_daily_domains
		 WHERE `+strings.Join(where, " AND ")+`
		 GROUP BY dest_host ORDER BY SUM(hits) DESC, dest_host ASC LIMIT ?`,
		append(args, limit)...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	hosts := []TopHost{}
	for rows.Next() {
		var h TopHost
		if err := rows.Scan(&h.DestHost, &h.Hits); err != nil {
			return nil, mapErr(err)
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}
