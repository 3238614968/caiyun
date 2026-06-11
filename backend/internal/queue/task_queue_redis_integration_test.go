package queue

import (
	"os"
	"strconv"
	"testing"
	"time"

	"caiyun/internal/cache"
)

func TestTaskQueueRedisIntegrationReliableLifecycle(t *testing.T) {
	if os.Getenv("CAIYUN_REDIS_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_REDIS_INTEGRATION=1 to run real Redis integration tests")
	}

	tests := []struct {
		name  string
		build func(*cache.RedisCache) ReliableTaskQueue
	}{
		{name: "list", build: func(redisCache *cache.RedisCache) ReliableTaskQueue {
			return NewTaskQueue(redisCache)
		}},
		{name: "streams", build: func(redisCache *cache.RedisCache) ReliableTaskQueue {
			return NewStreamTaskQueue(redisCache, StreamTaskQueueOptions{
				ConsumerName: "integration-test",
			})
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redisCache := newIntegrationRedisCache(t)
			q := tt.build(redisCache)

			if err := q.Clear(); err != nil {
				t.Fatalf("Clear() before test error = %v", err)
			}
			t.Cleanup(func() {
				_ = q.Clear()
				_ = redisCache.Close()
			})

			if err := q.Enqueue(1001, 2002, "signin"); err != nil {
				t.Fatalf("Enqueue() error = %v", err)
			}
			assertQueueLen(t, q.GetQueueLength, 1, "pending after enqueue")

			msg, err := q.Dequeue(time.Second)
			if err != nil {
				t.Fatalf("Dequeue() error = %v", err)
			}
			if msg.AccountID != 1001 || msg.UserID != 2002 || msg.TaskType != "signin" {
				t.Fatalf("dequeued message = %+v, want account/user/type", msg)
			}
			if msg.ProcessingAt == 0 || msg.raw == "" {
				t.Fatalf("dequeued message should carry processing metadata: %+v", msg)
			}
			assertQueueLen(t, q.GetQueueLength, 0, "pending after dequeue")
			assertQueueLen(t, q.GetProcessingLength, 1, "processing after dequeue")

			recovered, err := q.RecoverStaleProcessing(time.Hour)
			if err != nil {
				t.Fatalf("RecoverStaleProcessing() error = %v", err)
			}
			if recovered != 0 {
				t.Fatalf("RecoverStaleProcessing() recovered = %d, want 0 for fresh message", recovered)
			}

			msg.RetryCount = 1
			if err := q.RequeueDelayed(msg, time.Second); err != nil {
				t.Fatalf("RequeueDelayed() error = %v", err)
			}
			assertQueueLen(t, q.GetProcessingLength, 0, "processing after delayed requeue")
			assertQueueLen(t, q.GetDelayedLength, 1, "delayed after delayed requeue")

			promoted := waitPromoteDelayed(t, q, 3*time.Second)
			if promoted != 1 {
				t.Fatalf("promoted = %d, want 1", promoted)
			}
			assertQueueLen(t, q.GetDelayedLength, 0, "delayed after promote")
			assertQueueLen(t, q.GetQueueLength, 1, "pending after promote")

			msg, err = q.Dequeue(time.Second)
			if err != nil {
				t.Fatalf("Dequeue() after promote error = %v", err)
			}
			if err := q.DeadLetter(msg, "integration failure sample"); err != nil {
				t.Fatalf("DeadLetter() error = %v", err)
			}
			assertQueueLen(t, q.GetProcessingLength, 0, "processing after dead letter")
			assertQueueLen(t, q.GetDeadLetterLength, 1, "dead letter after dead letter")

			if err := q.Enqueue(1003, 2004, "all"); err != nil {
				t.Fatalf("Enqueue() for ack error = %v", err)
			}
			msg, err = q.Dequeue(time.Second)
			if err != nil {
				t.Fatalf("Dequeue() for ack error = %v", err)
			}
			if err := q.Ack(msg); err != nil {
				t.Fatalf("Ack() error = %v", err)
			}
			assertQueueLen(t, q.GetProcessingLength, 0, "processing after ack")
		})
	}
}

func newIntegrationRedisCache(t *testing.T) *cache.RedisCache {
	t.Helper()

	addr := getenv("CAIYUN_TEST_REDIS_ADDR", "127.0.0.1:6379")
	password := os.Getenv("CAIYUN_TEST_REDIS_PASSWORD")
	dbRaw := getenv("CAIYUN_TEST_REDIS_DB", "15")
	db, err := strconv.Atoi(dbRaw)
	if err != nil {
		t.Fatalf("invalid CAIYUN_TEST_REDIS_DB=%q: %v", dbRaw, err)
	}

	redisCache, err := cache.NewRedisClient(addr, password, db)
	if err != nil {
		t.Fatalf("connect Redis %s db %d error = %v", addr, db, err)
	}
	return redisCache
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func assertQueueLen(t *testing.T, getter func() (int64, error), want int64, label string) {
	t.Helper()
	got, err := getter()
	if err != nil {
		t.Fatalf("%s length error = %v", label, err)
	}
	if got != want {
		t.Fatalf("%s length = %d, want %d", label, got, want)
	}
}

func waitPromoteDelayed(t *testing.T, q ReliableTaskQueue, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var promoted int
	for time.Now().Before(deadline) {
		got, err := q.PromoteDueDelayed(10)
		if err != nil {
			t.Fatalf("PromoteDueDelayed() error = %v", err)
		}
		promoted += got
		if promoted > 0 {
			return promoted
		}
		time.Sleep(200 * time.Millisecond)
	}
	return promoted
}
