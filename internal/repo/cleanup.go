package repo

import (
	"context"
	"time"
)

func (r *Repo) DeleteTrafficRecordsBatch(ctx context.Context, cutoff time.Time, limit int64) (int64, error) {
	return r.execDeleteBatch(ctx,
		`DELETE FROM traffic_records WHERE id IN (SELECT id FROM traffic_records WHERE created_at < ? LIMIT ?)`,
		cutoff.Unix(), limit)
}

func (r *Repo) DeleteExpiredAdminSessionsBatch(ctx context.Context, now time.Time, limit int64) (int64, error) {
	return r.execDeleteBatch(ctx,
		`DELETE FROM admin_sessions WHERE rowid IN (SELECT rowid FROM admin_sessions WHERE expires_at < ? LIMIT ?)`,
		now.Unix(), limit)
}

func (r *Repo) DeleteTrafficBatchesBatch(ctx context.Context, cutoff time.Time, limit int64) (int64, error) {
	return r.execDeleteBatch(ctx,
		`DELETE FROM traffic_batches WHERE rowid IN (SELECT rowid FROM traffic_batches WHERE received_at < ? LIMIT ?)`,
		cutoff.Unix(), limit)
}

func (r *Repo) DeleteDeviceBatchesBatch(ctx context.Context, cutoff time.Time, limit int64) (int64, error) {
	return r.execDeleteBatch(ctx,
		`DELETE FROM device_batches WHERE rowid IN (SELECT rowid FROM device_batches WHERE received_at < ? LIMIT ?)`,
		cutoff.Unix(), limit)
}

func (r *Repo) DeleteVisitRecordsBatch(ctx context.Context, cutoff time.Time, limit int64) (int64, error) {
	return r.execDeleteBatch(ctx,
		`DELETE FROM visit_records WHERE id IN (SELECT id FROM visit_records WHERE created_at < ? LIMIT ?)`,
		cutoff.Unix(), limit)
}

func (r *Repo) DeleteVisitDailyDomainsBatch(ctx context.Context, cutoff time.Time, limit int64) (int64, error) {
	return r.execDeleteBatch(ctx,
		`DELETE FROM visit_daily_domains WHERE rowid IN (SELECT rowid FROM visit_daily_domains WHERE day < ? LIMIT ?)`,
		cutoff.Unix(), limit)
}

func (r *Repo) DeleteVisitBatchesBatch(ctx context.Context, cutoff time.Time, limit int64) (int64, error) {
	return r.execDeleteBatch(ctx,
		`DELETE FROM visit_batches WHERE rowid IN (SELECT rowid FROM visit_batches WHERE received_at < ? LIMIT ?)`,
		cutoff.Unix(), limit)
}

func (r *Repo) CountTrafficRecords(ctx context.Context) (int64, error) {
	return r.execCount(ctx, `SELECT COUNT(*) FROM traffic_records`)
}

func (r *Repo) DeleteTrafficRecordsBeyondCap(ctx context.Context, keep, limit int64) (int64, error) {
	return r.execDeleteBatch(ctx,
		`DELETE FROM traffic_records WHERE id IN (SELECT id FROM traffic_records ORDER BY id DESC LIMIT ? OFFSET ?)`,
		limit, keep)
}

func (r *Repo) execCount(ctx context.Context, query string) (int64, error) {
	var n int64
	if err := r.DB.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return 0, mapErr(err)
	}
	return n, nil
}

func (r *Repo) execDeleteBatch(ctx context.Context, query string, args ...any) (int64, error) {
	res, err := r.DB.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, mapErr(err)
	}
	return res.RowsAffected()
}
