package janitor

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"vps-node/internal/repo"
)

const (
	defaultBatchSize   = int64(500)
	staleDeviceHorizon = 24 * time.Hour

	settingRetentionAggregate      = "retention.aggregate_days"
	settingRetentionVisit          = "retention.visit_days"
	settingRetentionVisitAggregate = "retention.visit_aggregate_days"
)

type Janitor struct {
	repo              *repo.Repo
	interval          time.Duration
	batchSize         int64
	logger            *slog.Logger
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

func (j *Janitor) WithCaps(maxTrafficRecords int64) *Janitor {
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
	aggDays, err := j.retentionDays(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	aggCutoff := now.AddDate(0, 0, -aggDays)

	trafficDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteTrafficRecordsBatch(ctx, aggCutoff, limit)
	})
	if err != nil {
		return err
	}
	devicesDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteStaleDevicesBatch(ctx, now.Add(-staleDeviceHorizon), limit)
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
	deviceBatchesDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteDeviceBatchesBatch(ctx, aggCutoff, limit)
	})
	if err != nil {
		return err
	}
	visitDays, err := j.daysSetting(ctx, settingRetentionVisit, 7)
	if err != nil {
		return err
	}
	visitAggregateDays, err := j.daysSetting(ctx, settingRetentionVisitAggregate, 90)
	if err != nil {
		return err
	}
	visitCutoff := now.AddDate(0, 0, -visitDays)
	visitAggCutoff := now.AddDate(0, 0, -visitAggregateDays)
	visitRecordsDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteVisitRecordsBatch(ctx, visitCutoff, limit)
	})
	if err != nil {
		return err
	}
	visitDailyDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteVisitDailyDomainsBatch(ctx, visitAggCutoff, limit)
	})
	if err != nil {
		return err
	}
	visitBatchesDeleted, err := j.drain(ctx, func(limit int64) (int64, error) {
		return j.repo.DeleteVisitBatchesBatch(ctx, visitAggCutoff, limit)
	})
	if err != nil {
		return err
	}
	trafficCapped, err := j.enforceTrafficRecordsCap(ctx)
	if err != nil {
		return err
	}

	j.logger.Debug("retention sweep completed",
		"aggregate_days", aggDays,
		"traffic_records_deleted", trafficDeleted,
		"devices_deleted", devicesDeleted,
		"admin_sessions_deleted", adminSessionsDeleted,
		"traffic_batches_deleted", trafficBatchesDeleted,
		"device_batches_deleted", deviceBatchesDeleted,
		"visit_records_deleted", visitRecordsDeleted,
		"visit_daily_domains_deleted", visitDailyDeleted,
		"visit_batches_deleted", visitBatchesDeleted,
		"traffic_records_cap_deleted", trafficCapped)
	return nil
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

func (j *Janitor) retentionDays(ctx context.Context) (int, error) {
	aggDays, err := j.daysSetting(ctx, settingRetentionAggregate, 90)
	if err != nil {
		return 0, err
	}
	return aggDays, nil
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
