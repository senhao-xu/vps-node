package repo

import (
	"context"
	"database/sql"
	"time"
)

// ServerCycleStart returns the current UTC monthly boundary. A reset day beyond
// a month's length falls on that month's last day.
func ServerCycleStart(now time.Time, resetDay int) time.Time {
	now = now.UTC()
	if resetDay < 1 || resetDay > 31 {
		resetDay = 1
	}
	boundary := func(year int, month time.Month) time.Time {
		last := time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
		day := resetDay
		if day > last {
			day = last
		}
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	current := boundary(now.Year(), now.Month())
	if !now.Before(current) {
		return current
	}
	previous := time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	return boundary(previous.Year(), previous.Month())
}

func serverMonthlyRaw(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, serverID int64, since time.Time) (upload, download int64, err error) {
	err = q.QueryRowContext(ctx, `SELECT COALESCE(SUM(u), 0), COALESCE(SUM(d), 0) FROM traffic_records WHERE server_id = ? AND created_at >= ?`, serverID, since.Unix()).Scan(&upload, &download)
	return upload, download, mapErr(err)
}

func serverMonthlyTotal(s Server, upload, download int64) int64 {
	total := upload + download
	if s.TrafficAccounting == "max" && upload > download {
		total = upload
	}
	if s.TrafficAccounting == "max" && download >= upload {
		total = download
	}
	if total < 0 {
		return 0
	}
	return total
}

func (r *Repo) ServerMonthlyUsage(ctx context.Context, s Server, now time.Time) (upload, download, used int64, err error) {
	start := ServerCycleStart(now, s.TrafficResetDay)
	upload, download, err = serverMonthlyRaw(ctx, r.DB, s.ID, start)
	if err != nil {
		return 0, 0, 0, err
	}
	return upload, download, serverMonthlyTotal(s, upload, download), nil
}
