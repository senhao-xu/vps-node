package repo

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

type TrafficRecord struct {
	ID            int64
	UserID        int64
	NodeID        int64
	ServerID      int64
	UploadBytes   int64
	DownloadBytes int64
	CreatedAt     time.Time
}

type NewTrafficRecord struct {
	UserID        int64
	NodeID        int64
	ServerID      int64
	UploadBytes   int64
	DownloadBytes int64
	CreatedAt     time.Time
}

type TrafficFilter struct {
	UserID   int64
	NodeID   int64
	ServerID int64
	From     *time.Time
	To       *time.Time
}

func (r *Repo) InsertTrafficRecords(ctx context.Context, records []NewTrafficRecord) error {
	return Tx(ctx, r.DB, func(tx *sql.Tx) error {
		for _, rec := range records {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO traffic_records (user_id, node_id, server_id, upload_bytes, download_bytes, created_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				rec.UserID, rec.NodeID, rec.ServerID, rec.UploadBytes, rec.DownloadBytes, rec.CreatedAt.Unix()); err != nil {
				return mapErr(err)
			}
		}
		return nil
	})
}

func trafficWhere(f TrafficFilter) (string, []any) {
	where := []string{"1 = 1"}
	args := []any{}
	if f.UserID != 0 {
		where = append(where, "user_id = ?")
		args = append(args, f.UserID)
	}
	if f.NodeID != 0 {
		where = append(where, "node_id = ?")
		args = append(args, f.NodeID)
	}
	if f.ServerID != 0 {
		where = append(where, "server_id = ?")
		args = append(args, f.ServerID)
	}
	if f.From != nil {
		where = append(where, "created_at >= ?")
		args = append(args, f.From.Unix())
	}
	if f.To != nil {
		where = append(where, "created_at < ?")
		args = append(args, f.To.Unix())
	}
	return strings.Join(where, " AND "), args
}

func (r *Repo) SumTraffic(ctx context.Context, f TrafficFilter) (upload, download int64, err error) {
	whereSQL, args := trafficWhere(f)
	err = r.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
		 FROM traffic_records WHERE `+whereSQL, args...).Scan(&upload, &download)
	return upload, download, mapErr(err)
}

type TrafficBucket struct {
	BucketStart   time.Time
	UploadBytes   int64
	DownloadBytes int64
}

func (r *Repo) SumTrafficBuckets(ctx context.Context, f TrafficFilter, bucketSeconds int64) ([]TrafficBucket, error) {
	if bucketSeconds < 1 {
		bucketSeconds = 86400
	}
	whereSQL, args := trafficWhere(f)
	rows, err := r.DB.QueryContext(ctx,
		`SELECT (created_at / ?) * ? AS bucket_start, COALESCE(SUM(upload_bytes), 0), COALESCE(SUM(download_bytes), 0)
		 FROM traffic_records WHERE `+whereSQL+` GROUP BY bucket_start ORDER BY bucket_start`,
		append([]any{bucketSeconds, bucketSeconds}, args...)...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()

	buckets := []TrafficBucket{}
	for rows.Next() {
		var start int64
		var up, down int64
		if err := rows.Scan(&start, &up, &down); err != nil {
			return nil, mapErr(err)
		}
		buckets = append(buckets, TrafficBucket{BucketStart: toTime(start), UploadBytes: up, DownloadBytes: down})
	}
	return buckets, rows.Err()
}

func (r *Repo) DeleteTrafficRecordsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.DB.ExecContext(ctx, `DELETE FROM traffic_records WHERE created_at < ?`, cutoff.Unix())
	if err != nil {
		return 0, mapErr(err)
	}
	return res.RowsAffected()
}
