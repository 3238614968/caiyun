package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeHistoryArchiveRepo struct {
	taskMoves     []int64
	exchangeMoves []int64
	taskErr       error
	exchangeErr   error
	taskCalls     int
	exchangeCalls int
	taskCtx       context.Context
	exchangeCtx   context.Context
}

func (f *fakeHistoryArchiveRepo) ArchiveTaskLogsBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	f.taskCalls++
	f.taskCtx = ctx
	if f.taskErr != nil {
		return 0, f.taskErr
	}
	if len(f.taskMoves) == 0 {
		return 0, nil
	}
	value := f.taskMoves[0]
	if len(f.taskMoves) > 1 {
		f.taskMoves = f.taskMoves[1:]
	}
	return value, nil
}

func (f *fakeHistoryArchiveRepo) ArchiveExchangeRecordsBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	f.exchangeCalls++
	f.exchangeCtx = ctx
	if f.exchangeErr != nil {
		return 0, f.exchangeErr
	}
	if len(f.exchangeMoves) == 0 {
		return 0, nil
	}
	value := f.exchangeMoves[0]
	if len(f.exchangeMoves) > 1 {
		f.exchangeMoves = f.exchangeMoves[1:]
	}
	return value, nil
}

type fakeHistoryArchiveLockStore struct {
	allowLock    bool
	setCalls     int
	releaseCalls int
}

type fakeHistoryArchiveMetrics struct {
	batches []historyArchiveBatchMetric
}

type historyArchiveBatchMetric struct {
	table    string
	moved    int64
	duration time.Duration
}

func (f *fakeHistoryArchiveMetrics) RecordHistoryArchiveBatch(table string, moved int64, duration time.Duration) {
	f.batches = append(f.batches, historyArchiveBatchMetric{table: table, moved: moved, duration: duration})
}

func (f *fakeHistoryArchiveLockStore) SetNX(key string, value interface{}, expiration time.Duration) (bool, error) {
	f.setCalls++
	return f.allowLock, nil
}

func (f *fakeHistoryArchiveLockStore) DelIfValue(key, value string) (bool, error) {
	f.releaseCalls++
	return true, nil
}

func TestHistoryArchiveConfigFromEnv(t *testing.T) {
	t.Setenv("ARCHIVE_JOB_ENABLED", "false")
	t.Setenv("ARCHIVE_JOB_SCHEDULE", "15 1 * * *")
	t.Setenv("ARCHIVE_TASK_LOGS_BEFORE_DAYS", "45")
	t.Setenv("ARCHIVE_EXCHANGE_RECORDS_BEFORE_DAYS", "60")
	t.Setenv("ARCHIVE_BATCH_SIZE", "321")
	t.Setenv("ARCHIVE_MAX_BATCHES_PER_RUN", "7")
	t.Setenv("ARCHIVE_JOB_LOCK_TTL", "45m")

	cfg := HistoryArchiveConfigFromEnv()
	if cfg.Enabled {
		t.Fatal("expected ARCHIVE_JOB_ENABLED=false")
	}
	if cfg.Schedule != "15 1 * * *" || cfg.TaskLogsBeforeDays != 45 || cfg.ExchangeRecordsBeforeDays != 60 {
		t.Fatalf("unexpected archive config: %+v", cfg)
	}
	if cfg.BatchSize != 321 || cfg.MaxBatchesPerRun != 7 || cfg.LockTTL != 45*time.Minute {
		t.Fatalf("unexpected archive config limits: %+v", cfg)
	}
}

func TestHistoryArchiveServiceRunOnceAggregatesBatches(t *testing.T) {
	repo := &fakeHistoryArchiveRepo{taskMoves: []int64{2, 0}, exchangeMoves: []int64{1, 0}}
	service := NewHistoryArchiveService(repo, HistoryArchiveConfig{
		Enabled:                   true,
		TaskLogsBeforeDays:        90,
		ExchangeRecordsBeforeDays: 180,
		BatchSize:                 1000,
		MaxBatchesPerRun:          10,
	})

	result, err := service.RunOnce(context.Background(), time.Date(2026, 6, 27, 2, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.TaskLogsMoved != 2 || result.ExchangeRecordsMoved != 1 {
		t.Fatalf("RunOnce() moved = %+v, want task_logs=2 exchange_records=1", result)
	}
	if result.Iterations != 2 || result.HitBatchLimit {
		t.Fatalf("RunOnce() iterations/limit = %+v", result)
	}
	if repo.taskCalls != 2 || repo.exchangeCalls != 2 {
		t.Fatalf("repo calls task=%d exchange=%d, want 2/2", repo.taskCalls, repo.exchangeCalls)
	}
	if repo.taskCtx == nil || repo.exchangeCtx == nil {
		t.Fatal("expected RunOnce to pass context into repository methods")
	}
}

func TestHistoryArchiveServiceRunOnceHitsBatchLimit(t *testing.T) {
	repo := &fakeHistoryArchiveRepo{taskMoves: []int64{1, 1}, exchangeMoves: []int64{0, 0}}
	service := NewHistoryArchiveService(repo, HistoryArchiveConfig{
		Enabled:                   true,
		TaskLogsBeforeDays:        90,
		ExchangeRecordsBeforeDays: 180,
		BatchSize:                 1000,
		MaxBatchesPerRun:          2,
	})

	result, err := service.RunOnce(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !result.HitBatchLimit {
		t.Fatalf("expected HitBatchLimit=true, got %+v", result)
	}
}

func TestHistoryArchiveServiceSkipsWhenLockHeld(t *testing.T) {
	repo := &fakeHistoryArchiveRepo{taskMoves: []int64{5}, exchangeMoves: []int64{3}}
	lockStore := &fakeHistoryArchiveLockStore{allowLock: false}
	service := NewHistoryArchiveService(repo, HistoryArchiveConfig{Enabled: true})
	service.SetLockStore(lockStore)

	result, err := service.RunOnce(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.TaskLogsMoved != 0 || result.ExchangeRecordsMoved != 0 {
		t.Fatalf("expected zero moves when lock held, got %+v", result)
	}
	if repo.taskCalls != 0 || repo.exchangeCalls != 0 {
		t.Fatalf("repo should not be called when lock not acquired, got task=%d exchange=%d", repo.taskCalls, repo.exchangeCalls)
	}
}

func TestHistoryArchiveServiceReturnsRepositoryError(t *testing.T) {
	repo := &fakeHistoryArchiveRepo{taskErr: errors.New("archive failed")}
	service := NewHistoryArchiveService(repo, HistoryArchiveConfig{Enabled: true})

	if _, err := service.RunOnce(context.Background(), time.Now()); err == nil {
		t.Fatal("expected repository error")
	}
}

func TestHistoryArchiveServiceHonorsCanceledContext(t *testing.T) {
	repo := &fakeHistoryArchiveRepo{taskMoves: []int64{1}, exchangeMoves: []int64{1}}
	service := NewHistoryArchiveService(repo, HistoryArchiveConfig{Enabled: true})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := service.RunOnce(ctx, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunOnce() error = %v, want context.Canceled", err)
	}
	if repo.taskCalls != 0 || repo.exchangeCalls != 0 {
		t.Fatalf("canceled context should stop before repository calls, got task=%d exchange=%d", repo.taskCalls, repo.exchangeCalls)
	}
}

func TestHistoryArchiveServiceRecordsBatchMetrics(t *testing.T) {
	repo := &fakeHistoryArchiveRepo{taskMoves: []int64{2, 0}, exchangeMoves: []int64{1, 0}}
	metrics := &fakeHistoryArchiveMetrics{}
	service := NewHistoryArchiveService(repo, HistoryArchiveConfig{
		Enabled:                   true,
		TaskLogsBeforeDays:        90,
		ExchangeRecordsBeforeDays: 180,
		BatchSize:                 1000,
		MaxBatchesPerRun:          10,
	})
	service.SetMetrics(metrics)

	if _, err := service.RunOnce(context.Background(), time.Date(2026, 6, 28, 2, 0, 0, 0, time.Local)); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if got := len(metrics.batches); got != 4 {
		t.Fatalf("batch metric count = %d, want 4", got)
	}
	if metrics.batches[0].table != "task_logs" || metrics.batches[0].moved != 2 {
		t.Fatalf("first batch metric = %+v, want task_logs moved=2", metrics.batches[0])
	}
	if metrics.batches[1].table != "exchange_records" || metrics.batches[1].moved != 1 {
		t.Fatalf("second batch metric = %+v, want exchange_records moved=1", metrics.batches[1])
	}
	for i, batch := range metrics.batches {
		if batch.duration < 0 {
			t.Fatalf("batch %d duration = %v, want >= 0", i, batch.duration)
		}
	}
}
