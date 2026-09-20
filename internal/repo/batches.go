package repo

import (
	"context"
)

func (r *Repo) TrafficBatchExists(ctx context.Context, agentID, seq int64) (bool, error) {
	return r.batchExists(ctx, `SELECT COUNT(*) FROM traffic_batches WHERE agent_id = ? AND seq = ?`, agentID, seq)
}

func (r *Repo) RecordTrafficBatch(ctx context.Context, agentID, seq int64) error {
	return r.recordBatch(ctx, `INSERT INTO traffic_batches (agent_id, seq, received_at) VALUES (?, ?, ?)`, agentID, seq)
}

func (r *Repo) LogBatchExists(ctx context.Context, agentID, seq int64) (bool, error) {
	return r.batchExists(ctx, `SELECT COUNT(*) FROM connection_log_batches WHERE agent_id = ? AND seq = ?`, agentID, seq)
}

func (r *Repo) RecordLogBatch(ctx context.Context, agentID, seq int64) error {
	return r.recordBatch(ctx, `INSERT INTO connection_log_batches (agent_id, seq, received_at) VALUES (?, ?, ?)`, agentID, seq)
}

func (r *Repo) batchExists(ctx context.Context, query string, agentID, seq int64) (bool, error) {
	var n int64
	if err := r.DB.QueryRowContext(ctx, query, agentID, seq).Scan(&n); err != nil {
		return false, mapErr(err)
	}
	return n > 0, nil
}

func (r *Repo) recordBatch(ctx context.Context, query string, agentID, seq int64) error {
	_, err := r.DB.ExecContext(ctx, query, agentID, seq, nowUnix())
	return mapErr(err)
}
