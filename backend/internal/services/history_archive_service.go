package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"caiyun/internal/constants"
	"caiyun/internal/envutil"
)

const (
	defaultArchiveTaskLogsBeforeDays        = 90
	defaultArchiveExchangeRecordsBeforeDays = 180
	defaultArchiveMaxBatchesPerRun          = 100
	defaultArchiveJobLockTTL                = 30 * time.Minute
)

type historyArchiveRepository interface {
	ArchiveTaskLogsBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
	ArchiveExchangeRecordsBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error)
}

type historyArchiveLockStore interface {
	SetNX(key string, value interface{}, expiration time.Duration) (bool, error)
	DelIfValue(key, value string) (bool, error)
}

type historyArchiveMetrics interface {
	RecordHistoryArchiveBatch(table string, moved int64, duration time.Duration)
}

type HistoryArchiveConfig struct {
	Enabled                   bool
	Schedule                  string
	TaskLogsBeforeDays        int
	ExchangeRecordsBeforeDays int
	BatchSize                 int
	MaxBatchesPerRun          int
	LockTTL                   time.Duration
}

type HistoryArchiveRunResult struct {
	TaskLogsMoved        int64
	ExchangeRecordsMoved int64
	Iterations           int
	HitBatchLimit        bool
	Skipped              bool
	SkipReason           string
}

// HistoryArchiveService 将 task_logs / exchange_records 定期归档到 archive 表。
type HistoryArchiveService struct {
	repo      historyArchiveRepository
	cfg       HistoryArchiveConfig
	lockStore historyArchiveLockStore
	metrics   historyArchiveMetrics
	lockKey   string
	lockOwner string
}

func NewHistoryArchiveService(repo historyArchiveRepository, cfg HistoryArchiveConfig) *HistoryArchiveService {
	cfg = normalizeHistoryArchiveConfig(cfg)
	return &HistoryArchiveService{
		repo:      repo,
		cfg:       cfg,
		lockKey:   "history:archive:lock",
		lockOwner: randomArchiveLockValue(),
	}
}

func NewHistoryArchiveServiceFromEnv(repo historyArchiveRepository) *HistoryArchiveService {
	return NewHistoryArchiveService(repo, HistoryArchiveConfigFromEnv())
}

func (s *HistoryArchiveService) SetLockStore(lockStore historyArchiveLockStore) {
	if s == nil {
		return
	}
	s.lockStore = lockStore
}

func (s *HistoryArchiveService) SetMetrics(metrics historyArchiveMetrics) {
	if s == nil {
		return
	}
	s.metrics = metrics
}

func (s *HistoryArchiveService) Enabled() bool {
	return s != nil && s.cfg.Enabled
}

func (s *HistoryArchiveService) Schedule() string {
	if s == nil {
		return constants.ArchiveHistoryCron
	}
	return s.cfg.Schedule
}

func (s *HistoryArchiveService) RunOnce(ctx context.Context, now time.Time) (*HistoryArchiveRunResult, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("history archive service 未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if !s.cfg.Enabled {
		return &HistoryArchiveRunResult{Skipped: true, SkipReason: "disabled"}, nil
	}
	if now.IsZero() {
		now = time.Now()
	}

	release, locked, err := s.acquireLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		log.Println("【历史归档】检测到其他实例正在执行归档，跳过本轮")
		return &HistoryArchiveRunResult{Skipped: true, SkipReason: "lock-held"}, nil
	}
	defer release()

	repo := s.repo
	result := &HistoryArchiveRunResult{}
	lastTaskMoved := int64(0)
	lastRecordMoved := int64(0)
	taskCutoff := now.AddDate(0, 0, -s.cfg.TaskLogsBeforeDays)
	recordCutoff := now.AddDate(0, 0, -s.cfg.ExchangeRecordsBeforeDays)

	for i := 0; i < s.cfg.MaxBatchesPerRun; i++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Iterations++

		taskMoved, err := s.archiveTaskLogs(ctx, repo, taskCutoff)
		if err != nil {
			return result, fmt.Errorf("归档 task_logs 失败: %w", err)
		}
		recordMoved, err := s.archiveExchangeRecords(ctx, repo, recordCutoff)
		if err != nil {
			return result, fmt.Errorf("归档 exchange_records 失败: %w", err)
		}

		result.TaskLogsMoved += taskMoved
		result.ExchangeRecordsMoved += recordMoved
		lastTaskMoved = taskMoved
		lastRecordMoved = recordMoved
		if taskMoved == 0 && recordMoved == 0 {
			break
		}
	}

	if result.Iterations >= s.cfg.MaxBatchesPerRun && (lastTaskMoved > 0 || lastRecordMoved > 0) {
		result.HitBatchLimit = true
	}

	log.Printf("【历史归档】完成：task_logs=%d, exchange_records=%d, iterations=%d, hit_limit=%t",
		result.TaskLogsMoved, result.ExchangeRecordsMoved, result.Iterations, result.HitBatchLimit)
	return result, nil
}

func (s *HistoryArchiveService) archiveTaskLogs(ctx context.Context, repo historyArchiveRepository, cutoff time.Time) (int64, error) {
	if s.cfg.TaskLogsBeforeDays <= 0 {
		return 0, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	startedAt := time.Now()
	moved, err := repo.ArchiveTaskLogsBefore(ctx, cutoff, s.cfg.BatchSize)
	s.recordArchiveBatch("task_logs", moved, time.Since(startedAt))
	return moved, err
}

func (s *HistoryArchiveService) archiveExchangeRecords(ctx context.Context, repo historyArchiveRepository, cutoff time.Time) (int64, error) {
	if s.cfg.ExchangeRecordsBeforeDays <= 0 {
		return 0, nil
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	startedAt := time.Now()
	moved, err := repo.ArchiveExchangeRecordsBefore(ctx, cutoff, s.cfg.BatchSize)
	s.recordArchiveBatch("exchange_records", moved, time.Since(startedAt))
	return moved, err
}

func (s *HistoryArchiveService) recordArchiveBatch(table string, moved int64, duration time.Duration) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.RecordHistoryArchiveBatch(table, moved, duration)
}

func (s *HistoryArchiveService) acquireLock() (func(), bool, error) {
	if s.lockStore == nil {
		return func() {}, true, nil
	}
	owner := s.lockOwner
	if owner == "" {
		owner = randomArchiveLockValue()
		s.lockOwner = owner
	}
	locked, err := s.lockStore.SetNX(s.lockKey, owner, s.cfg.LockTTL)
	if err != nil {
		return nil, false, fmt.Errorf("获取历史归档分布式锁失败: %w", err)
	}
	if !locked {
		return func() {}, false, nil
	}
	return func() {
		if _, err := s.lockStore.DelIfValue(s.lockKey, owner); err != nil {
			log.Printf("【历史归档】释放分布式锁失败: %v", err)
		}
	}, true, nil
}

func HistoryArchiveConfigFromEnv() HistoryArchiveConfig {
	return normalizeHistoryArchiveConfig(HistoryArchiveConfig{
		Enabled:                   envutil.Bool("ARCHIVE_JOB_ENABLED", true),
		Schedule:                  envutil.String("ARCHIVE_JOB_SCHEDULE", constants.ArchiveHistoryCron),
		TaskLogsBeforeDays:        envutil.Int("ARCHIVE_TASK_LOGS_BEFORE_DAYS", defaultArchiveTaskLogsBeforeDays),
		ExchangeRecordsBeforeDays: envutil.Int("ARCHIVE_EXCHANGE_RECORDS_BEFORE_DAYS", defaultArchiveExchangeRecordsBeforeDays),
		BatchSize:                 envutil.Int("ARCHIVE_BATCH_SIZE", 1000),
		MaxBatchesPerRun:          envutil.Int("ARCHIVE_MAX_BATCHES_PER_RUN", defaultArchiveMaxBatchesPerRun),
		LockTTL:                   envutil.Duration("ARCHIVE_JOB_LOCK_TTL", defaultArchiveJobLockTTL),
	})
}

func normalizeHistoryArchiveConfig(cfg HistoryArchiveConfig) HistoryArchiveConfig {
	if cfg.Schedule == "" {
		cfg.Schedule = constants.ArchiveHistoryCron
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1000
	}
	if cfg.MaxBatchesPerRun <= 0 {
		cfg.MaxBatchesPerRun = defaultArchiveMaxBatchesPerRun
	}
	if cfg.LockTTL <= 0 {
		cfg.LockTTL = defaultArchiveJobLockTTL
	}
	if cfg.TaskLogsBeforeDays == 0 {
		cfg.TaskLogsBeforeDays = defaultArchiveTaskLogsBeforeDays
	}
	if cfg.ExchangeRecordsBeforeDays == 0 {
		cfg.ExchangeRecordsBeforeDays = defaultArchiveExchangeRecordsBeforeDays
	}
	return cfg
}

func randomArchiveLockValue() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("archive:%d", time.Now().UnixNano())
	}
	return "archive:" + hex.EncodeToString(buf[:])
}
