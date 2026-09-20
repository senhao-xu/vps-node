package janitor

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"vps-node/internal/repo"
)

const (
	defaultBatchSize    = int64(500)
	staleSessionHorizon = 24 * time.Hour

	settingRetentionRawLog    = "retention.raw_log_days"
	settingRetentionAggregate = "retention.aggregate_days"
)

type Janitor struct {
	repo              *repo.Repo
	interval          time.Duration
	batchSize         int64
	logger            *slog.Logger
	maxConnectionLogs int64
	maxTrafficRecords int64
}

func New(store *repo.Repo, interval time.Duration, logger *slog.Logger) *Janitor {
	if interval <= 0 {
		interval = time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Janitor{repo: store, interval: interval, batchSize: defaultBatchSize, logger: logger}
}

func (j *Janitor) WithCaps(maxConnectionLogs, maxTrafficRecords int64) *Janitor {
	j.maxConnectionLogs = max64(maxConnectionLogs, 0)
	j.maxTrafficRecords = max64(maxTrafficRecords, 0)
	return j
}

func (j *Janitor) Run(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := j.Sweep(ctx); err != nil {
				j.logger.Warn("retention sweep failed", "error", err)
			}
		}
	}
}

func (j *Janitor) Sweep(ctx context.Context) error {
	rawDays, aggDays, err := j.retentionDays(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	rawCutoff := now.AddDate(0, 0, -rawDays)
	aggCutoff := now.AddDate(0, 0, -aggDays)

	logsDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteConnectionLogsBatch(ctx, rawCutoff, limit)
	})
	if err != nil {
		return err
	}
	trafficDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteTrafficRecordsBatch(ctx, aggCutoff, limit)
	})
	if err != nil {
		return err
	}
	sessionsDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteSessionsLastSeenBeforeBatch(ctx, now.Add(-staleSessionHorizon), limit)
	})
	if err != nil {
		return err
	}
	adminSessionsDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteExpiredAdminSessionsBatch(ctx, now, limit)
	})
	if err != nil {
		return err
	}
	trafficBatchesDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteTrafficBatchesBatch(ctx, aggCutoff, limit)
	})
	if err != nil {
		return err
	}
	logBatchesDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteLogBatchesBatch(ctx, aggCutoff, limit)
	})
	if err != nil {
		return err
	}
	logsCapped, err := j.enforceConnectionLogsCap(ctx)
	if err != nil {
		return err
	}
	trafficCapped, err := j.enforceTrafficRecordsCap(ctx)
	if err != nil {
		return err
	}

	j.logger.Debug("retention sweep completed",
		"raw_log_days", rawDays,
		"aggregate_days", aggDays,
		"connection_logs_deleted", logsDeleted,
		"traffic_records_deleted", trafficDeleted,
		"sessions_deleted", sessionsDeleted,
		"admin_sessions_deleted", adminSessionsDeleted,
		"traffic_batches_deleted", trafficBatchesDeleted,
		"log_batches_deleted", logBatchesDeleted,
		"connection_logs_cap_deleted", logsCapped,
		"traffic_records_cap_deleted", trafficCapped)
	return nil
}

func (j *Janitor) enforceConnectionLogsCap(ctx context.Context) (int64, error) {
	if j.maxConnectionLogs <= 0 {
		return 0, nil
	}
	count, err := j.repo.CountConnectionLogs(ctx)
	if err != nil {
		return 0, err
	}
	if count <= j.maxConnectionLogs {
		return 0, nil
	}
	return j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteConnectionLogsBeyondCap(ctx, j.maxConnectionLogs, limit)
	})
}

func (j *Janitor) enforceTrafficRecordsCap(ctx context.Context) (int64, error) {
	if j.maxTrafficRecords <= 0 {
		return 0, nil
	}
	count, err := j.repo.CountTrafficRecords(ctx)
	if err != nil {
		return 0, err
	}
	if count <= j.maxTrafficRecords {
		return 0, nil
	}
	return j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteTrafficRecordsBeyondCap(ctx, j.maxTrafficRecords, limit)
	})
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (j *Janitor) drain(ctx context.Context, deleteBatch func(limit int64) (int64, error)) (int64, error) {
	var total int64
	for {
		n, err := deleteBatch(j.batchSize)
		if err != nil {
			return total, err
		}
		total += n
		if n < j.batchSize {
			return total, nil
		}
	}
}

func (j *Janitor) retentionDays(ctx context.Context) (int, int, error) {
	rawDays, err := j.daysSetting(ctx, settingRetentionRawLog, 7)
	if err != nil {
		return 0, 0, err
	}
	aggDays, err := j.daysSetting(ctx, settingRetentionAggregate, 90)
	if err != nil {
		return 0, 0, err
	}
	return rawDays, aggDays, nil
}

func (j *Janitor) daysSetting(ctx context.Context, key string, fallback int) (int, error) {
	raw, err := j.repo.GetSettingOr(ctx, key, strconv.Itoa(fallback))
	if err != nil {
		return fallback, err
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback, nil
	}
	return n, nil
}
