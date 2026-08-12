//go:build cgo

package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/repository"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestOperationMySQLRedisIntegrationCancelAndReplayRaces exercises the exact
// deployment persistence pair. SQLite unit tests cover state-machine branches;
// this matrix verifies MySQL compare-and-swap plus both Redis queue backends.
func TestOperationMySQLRedisIntegrationCancelAndReplayRaces(t *testing.T) {
	if os.Getenv("CAIYUN_OPERATION_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_OPERATION_INTEGRATION=1 to run MySQL/Redis operation integration tests")
	}

	db := openOperationMySQLIntegrationDB(t)
	redisCache := newOperationRedisIntegrationCache(t)
	t.Cleanup(func() {
		_ = redisCache.Close()
	})

	for _, backend := range operationIntegrationQueueBackends() {
		t.Run(backend.name, func(t *testing.T) {
			q := backend.build(redisCache)
			if err := q.Clear(); err != nil {
				t.Fatalf("clear queue before test: %v", err)
			}
			t.Cleanup(func() { _ = q.Clear() })

			service := NewOperationService(repository.NewOperationRepository(db), q)
			assertOperationCancelClaimRace(t, service, q)
			resetOperationIntegrationCase(t, db, q)
			assertOperationDuplicateDeliveryFencing(t, service)
			resetOperationIntegrationCase(t, db, q)
			assertOperationDeadLetterReplayRace(t, service, q)
		})
	}
}

type operationIntegrationQueueBackend struct {
	name  string
	build func(*cache.RedisCache) queue.ReliableTaskQueue
}

func operationIntegrationQueueBackends() []operationIntegrationQueueBackend {
	suffix := fmt.Sprintf("operation-integration-%d", time.Now().UnixNano())
	return []operationIntegrationQueueBackend{
		{name: "list", build: func(redisCache *cache.RedisCache) queue.ReliableTaskQueue {
			return queue.NewTaskQueue(redisCache)
		}},
		{name: "streams", build: func(redisCache *cache.RedisCache) queue.ReliableTaskQueue {
			return queue.NewStreamTaskQueue(redisCache, queue.StreamTaskQueueOptions{
				StreamKey:     queue.TaskStreamKey + ":" + suffix,
				DelayedKey:    queue.TaskStreamDelayedKey + ":" + suffix,
				DeadLetterKey: queue.TaskStreamDeadLetterKey + ":" + suffix,
				ConsumerGroup: queue.DefaultStreamConsumerGroup + ":" + suffix,
				ConsumerName:  "operation-matrix",
			})
		}},
	}
}

func resetOperationIntegrationCase(t *testing.T, db *gorm.DB, q queue.ReliableTaskQueue) {
	t.Helper()
	if err := q.Clear(); err != nil {
		t.Fatalf("clear queue between matrix cases: %v", err)
	}
	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&models.Operation{}).Error; err != nil {
		t.Fatalf("clear operations between matrix cases: %v", err)
	}
}

// assertOperationDuplicateDeliveryFencing simulates a visibility-timeout
// redelivery. The first worker lease expires, a second worker reclaims the
// same Operation, and the old token must be unable to commit a terminal state.
func assertOperationDuplicateDeliveryFencing(t *testing.T, service *OperationService) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, time.July, 29, 0, 0, 0, 0, time.UTC)
	now := base
	originalNow := service.now
	service.now = func() time.Time { return now }
	t.Cleanup(func() { service.now = originalNow })

	op, created, dispatchErr, err := service.Submit(ctx, SubmitOperationRequest{
		UserID:         503,
		OperationType:  models.OperationTypeAccountTask,
		AccountID:      703,
		Payload:        AccountTaskOperationPayload{AccountID: 703, TaskType: "all_tasks"},
		IdempotencyKey: "mysql-redis-duplicate-delivery",
	})
	if err != nil || dispatchErr != nil || !created {
		t.Fatalf("submit duplicate-delivery operation: op=%+v created=%t dispatch=%v err=%v", op, created, dispatchErr, err)
	}

	first, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed || first.ExecutionToken == "" {
		t.Fatalf("first duplicate delivery claim: op=%+v claimed=%t err=%v", first, claimed, err)
	}
	firstToken := first.ExecutionToken

	// The same persisted message becomes visible again after its lease timeout.
	now = now.Add(2 * time.Minute)
	second, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed || second.ExecutionToken == "" || second.ExecutionToken == firstToken {
		t.Fatalf("redelivered claim must receive a new fencing token: op=%+v claimed=%t err=%v", second, claimed, err)
	}

	if err := service.MarkSucceeded(ctx, op.ID, firstToken); !errors.Is(err, repository.ErrOperationExecutionLost) {
		t.Fatalf("stale delivery terminal write err=%v, want execution loss", err)
	}
	if err := service.MarkSucceeded(ctx, op.ID, second.ExecutionToken); err != nil {
		t.Fatalf("current delivery terminal write: %v", err)
	}
	final, err := service.Get(ctx, op.ID)
	if err != nil || final.Status != models.OperationSucceeded || final.ExecutionToken != "" {
		t.Fatalf("duplicate delivery final state: op=%+v err=%v", final, err)
	}
}

func assertOperationCancelClaimRace(t *testing.T, service *OperationService, q queue.ReliableTaskQueue) {
	t.Helper()
	ctx := context.Background()
	op, created, dispatchErr, err := service.Submit(ctx, SubmitOperationRequest{
		UserID:         501,
		OperationType:  models.OperationTypeAccountTask,
		AccountID:      701,
		Payload:        AccountTaskOperationPayload{AccountID: 701, TaskType: "all_tasks"},
		IdempotencyKey: "mysql-redis-cancel-race",
	})
	if err != nil || dispatchErr != nil || !created {
		t.Fatalf("submit cancel race: op=%+v created=%t dispatch=%v err=%v", op, created, dispatchErr, err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var claimed bool
	var claimErr error
	var cancelErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, claimed, claimErr = service.TryStart(ctx, op.ID, time.Minute)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, cancelErr = service.Cancel(ctx, op.ID, 501)
	}()
	close(start)
	wg.Wait()

	final, err := service.Get(ctx, op.ID)
	if err != nil {
		t.Fatalf("load cancel-race operation: %v", err)
	}
	switch final.Status {
	case models.OperationCanceled:
		if claimed || claimErr != nil || cancelErr != nil {
			t.Fatalf("cancel winner must block claim: claimed=%t claimErr=%v cancelErr=%v final=%+v", claimed, claimErr, cancelErr, final)
		}
	case models.OperationRunning:
		if !claimed || claimErr != nil || !errors.Is(cancelErr, ErrOperationNotCancelable) || final.ExecutionToken == "" {
			t.Fatalf("claim winner must retain lease: claimed=%t claimErr=%v cancelErr=%v final=%+v", claimed, claimErr, cancelErr, final)
		}
	default:
		t.Fatalf("cancel/claim race reached illegal state: %+v", final)
	}

	// A consumed Redis message may still exist after a cancellation race. The
	// worker treats the database state as authoritative and must skip it.
	message, dequeueErr := q.Dequeue(time.Second)
	if dequeueErr != nil {
		t.Fatalf("dequeue cancel-race operation: %v", dequeueErr)
	}
	if message.OperationID != op.ID {
		t.Fatalf("dequeued operation id=%q, want %q", message.OperationID, op.ID)
	}
	if err := q.Ack(message); err != nil {
		t.Fatalf("ack cancel-race operation: %v", err)
	}
}

func assertOperationDeadLetterReplayRace(t *testing.T, service *OperationService, q queue.ReliableTaskQueue) {
	t.Helper()
	ctx := context.Background()
	op, created, dispatchErr, err := service.Submit(ctx, SubmitOperationRequest{
		UserID:         502,
		OperationType:  models.OperationTypeExchangeTask,
		ResourceID:     702,
		Payload:        ExchangeTaskOperationPayload{TaskID: 702},
		IdempotencyKey: "mysql-redis-replay-race",
	})
	if err != nil || dispatchErr != nil || !created {
		t.Fatalf("submit replay race: op=%+v created=%t dispatch=%v err=%v", op, created, dispatchErr, err)
	}
	message, err := q.Dequeue(time.Second)
	if err != nil {
		t.Fatalf("dequeue replay-race operation: %v", err)
	}
	running, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed || running.ExecutionToken == "" {
		t.Fatalf("claim replay-race operation: running=%+v claimed=%t err=%v", running, claimed, err)
	}
	if err := service.MarkFailed(ctx, op.ID, running.ExecutionToken, errors.New("integration upstream failure")); err != nil {
		t.Fatalf("mark failed replay-race operation: %v", err)
	}
	if err := q.DeadLetter(message, "operation integration replay race"); err != nil {
		t.Fatalf("dead-letter replay-race operation: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, replayErr := service.ReplayFailed(ctx, op.ID)
			results <- replayErr
		}()
	}
	close(start)
	var successes int
	var rejected int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrOperationNotReplayable):
			rejected++
		default:
			t.Fatalf("unexpected replay race result: %v", err)
		}
	}
	if successes != 1 || rejected != 1 {
		t.Fatalf("replay race outcomes: successes=%d rejected=%d", successes, rejected)
	}
	final, err := service.Get(ctx, op.ID)
	if err != nil || final.Status != models.OperationQueued || final.ExecutionToken != "" {
		t.Fatalf("replayed operation state: %+v err=%v", final, err)
	}
	length, err := q.GetQueueLength()
	if err != nil || length != 1 {
		t.Fatalf("replay must enqueue exactly once: length=%d err=%v", length, err)
	}
	replayedMessage, err := q.Dequeue(time.Second)
	if err != nil || replayedMessage.OperationID != op.ID {
		t.Fatalf("dequeue replayed operation: message=%+v err=%v", replayedMessage, err)
	}
	if err := q.Ack(replayedMessage); err != nil {
		t.Fatalf("ack replayed operation: %v", err)
	}
}

func newOperationRedisIntegrationCache(t *testing.T) *cache.RedisCache {
	t.Helper()
	addr := operationIntegrationEnv("CAIYUN_TEST_REDIS_ADDR", "127.0.0.1:6379")
	redisDB := 15
	if raw := strings.TrimSpace(os.Getenv("CAIYUN_TEST_REDIS_DB")); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &redisDB); err != nil {
			t.Fatalf("parse CAIYUN_TEST_REDIS_DB=%q: %v", raw, err)
		}
	}
	redisCache, err := cache.NewRedisClient(addr, os.Getenv("CAIYUN_TEST_REDIS_PASSWORD"), redisDB)
	if err != nil {
		t.Fatalf("connect Redis %s db %d: %v", addr, redisDB, err)
	}
	return redisCache
}

func openOperationMySQLIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	addr := operationIntegrationEnv("CAIYUN_TEST_MYSQL_ADDR", "127.0.0.1:3306")
	user := operationIntegrationEnv("CAIYUN_TEST_MYSQL_USER", "root")
	password := os.Getenv("CAIYUN_TEST_MYSQL_PASSWORD")
	admin := openOperationMySQL(t, operationMySQLDSN(user, password, addr, ""))
	name := fmt.Sprintf("caiyun_operation_it_%d", time.Now().UnixNano())
	quotedName := "`" + strings.ReplaceAll(name, "`", "") + "`"
	if err := admin.Exec("CREATE DATABASE " + quotedName + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatalf("create operation integration database: %v", err)
	}
	t.Cleanup(func() {
		_ = admin.Exec("DROP DATABASE IF EXISTS " + quotedName).Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	db := openOperationMySQL(t, operationMySQLDSN(user, password, addr, name))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Operation{}); err != nil {
		t.Fatalf("migrate operations table: %v", err)
	}
	return db
}

func openOperationMySQL(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), PrepareStmt: true})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL connection: %v", err)
	}
	sqlDB.SetConnMaxLifetime(time.Minute)
	sqlDB.SetMaxOpenConns(6)
	sqlDB.SetMaxIdleConns(6)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping MySQL: %v", err)
	}
	return db
}

func operationMySQLDSN(user, password, addr, database string) string {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/", user, password, addr)
	if database != "" {
		dsn += database
	}
	return dsn + "?charset=utf8mb4&parseTime=True&loc=UTC"
}

func operationIntegrationEnv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
