package queue

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"caiyun/internal/cache"
)

func TestTaskQueueRedisIntegrationReliableLifecycle(t *testing.T) {
	if os.Getenv("CAIYUN_REDIS_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_REDIS_INTEGRATION=1 to run real Redis integration tests")
	}

	for _, tt := range integrationQueueBackends() {
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

func TestTaskQueueRedisIntegrationConcurrentDuplicateMessages(t *testing.T) {
	if os.Getenv("CAIYUN_REDIS_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_REDIS_INTEGRATION=1 to run real Redis integration tests")
	}

	for _, tt := range integrationQueueBackends() {
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

			const duplicates = 32
			for i := 0; i < duplicates; i++ {
				if err := q.Enqueue(9001, 42, "duplicate"); err != nil {
					t.Fatalf("Enqueue duplicate %d error = %v", i, err)
				}
			}

			var wg sync.WaitGroup
			seen := make(chan string, duplicates)
			worker := func(id int) {
				defer wg.Done()
				for {
					msg, err := q.Dequeue(250 * time.Millisecond)
					if err == ErrQueueTimeout {
						return
					}
					if err != nil {
						seen <- fmt.Sprintf("error:%v", err)
						return
					}
					if msg.AccountID != 9001 || msg.UserID != 42 || msg.TaskType != "duplicate" {
						seen <- fmt.Sprintf("bad:%+v", msg)
						_ = q.DeadLetter(msg, "bad message in duplicate test")
						continue
					}
					if err := q.Ack(msg); err != nil {
						seen <- fmt.Sprintf("ack:%v", err)
						return
					}
					seen <- fmt.Sprintf("ok:%d", id)
				}
			}

			for i := 0; i < 6; i++ {
				wg.Add(1)
				go worker(i)
			}
			wg.Wait()
			close(seen)

			processed := 0
			for item := range seen {
				if item[:2] != "ok" {
					t.Fatalf("worker returned %s", item)
				}
				processed++
			}
			if processed != 1 {
				t.Fatalf("processed duplicates = %d, want %d after enqueue dedupe", processed, 1)
			}
			assertQueueLen(t, q.GetQueueLength, 0, "pending after duplicate drain")
			assertQueueLen(t, q.GetProcessingLength, 0, "processing after duplicate drain")
		})
	}
}

func TestTaskQueueRedisIntegrationCrashRecoveryAndDelayedBatch(t *testing.T) {
	if os.Getenv("CAIYUN_REDIS_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_REDIS_INTEGRATION=1 to run real Redis integration tests")
	}

	for _, tt := range integrationQueueBackends() {
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

			if err := q.Enqueue(7001, 100, "crash-sim"); err != nil {
				t.Fatalf("Enqueue crash-sim error = %v", err)
			}
			msg, err := q.Dequeue(time.Second)
			if err != nil {
				t.Fatalf("Dequeue crash-sim error = %v", err)
			}
			if msg == nil {
				t.Fatalf("Dequeue crash-sim returned nil")
			}
			time.Sleep(1200 * time.Millisecond)
			recovered, err := q.RecoverStaleProcessing(time.Second)
			if err != nil {
				t.Fatalf("RecoverStaleProcessing crash-sim error = %v", err)
			}
			if recovered != 1 {
				t.Fatalf("recovered = %d, want 1", recovered)
			}
			assertQueueLen(t, q.GetQueueLength, 1, "pending after crash recovery")
			msg, err = q.Dequeue(time.Second)
			if err != nil {
				t.Fatalf("Dequeue recovered crash-sim error = %v", err)
			}
			if err := q.Ack(msg); err != nil {
				t.Fatalf("Ack recovered crash-sim error = %v", err)
			}

			const batch = 25
			for i := 0; i < batch; i++ {
				if err := q.Enqueue(uint(8000+i), 101, "delayed-batch"); err != nil {
					t.Fatalf("Enqueue delayed-batch %d error = %v", i, err)
				}
			}
			for i := 0; i < batch; i++ {
				msg, err := q.Dequeue(time.Second)
				if err != nil {
					t.Fatalf("Dequeue delayed-batch %d error = %v", i, err)
				}
				msg.RetryCount = 1
				if err := q.RequeueDelayed(msg, time.Second); err != nil {
					t.Fatalf("RequeueDelayed batch %d error = %v", i, err)
				}
			}
			assertQueueLen(t, q.GetDelayedLength, batch, "delayed before batch promote")

			deadline := time.Now().Add(5 * time.Second)
			promoted := 0
			for promoted < batch && time.Now().Before(deadline) {
				got, err := q.PromoteDueDelayed(7)
				if err != nil {
					t.Fatalf("PromoteDueDelayed batch error = %v", err)
				}
				promoted += got
				if promoted < batch {
					time.Sleep(200 * time.Millisecond)
				}
			}
			if promoted != batch {
				t.Fatalf("promoted batch = %d, want %d", promoted, batch)
			}
			assertQueueLen(t, q.GetDelayedLength, 0, "delayed after batch promote")
			assertQueueLen(t, q.GetQueueLength, batch, "pending after batch promote")
		})
	}
}

func TestTaskQueueRedisIntegrationConcurrentDelayedPromotersNoDuplicates(t *testing.T) {
	if os.Getenv("CAIYUN_REDIS_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_REDIS_INTEGRATION=1 to run real Redis integration tests")
	}

	for _, tt := range integrationQueueBackends() {
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

			const batch = 36
			for i := 0; i < batch; i++ {
				if err := q.Enqueue(uint(9300+i), 707, "concurrent-promote"); err != nil {
					t.Fatalf("Enqueue concurrent-promote %d error = %v", i, err)
				}
			}
			for i := 0; i < batch; i++ {
				msg, err := q.Dequeue(time.Second)
				if err != nil {
					t.Fatalf("Dequeue concurrent-promote %d error = %v", i, err)
				}
				msg.RetryCount = 1
				if err := q.RequeueDelayed(msg, time.Second); err != nil {
					t.Fatalf("RequeueDelayed concurrent-promote %d error = %v", i, err)
				}
			}
			assertQueueLen(t, q.GetDelayedLength, batch, "delayed before concurrent promote")

			time.Sleep(1200 * time.Millisecond)
			var wg sync.WaitGroup
			errCh := make(chan error, 4)
			promotedCh := make(chan int, 4)
			promoter := func() {
				defer wg.Done()
				localPromoted := 0
				idleRounds := 0
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					got, err := q.PromoteDueDelayed(3)
					if err != nil {
						errCh <- err
						return
					}
					if got == 0 {
						idleRounds++
						if idleRounds >= 3 {
							break
						}
						time.Sleep(100 * time.Millisecond)
						continue
					}
					idleRounds = 0
					localPromoted += got
				}
				promotedCh <- localPromoted
			}

			for i := 0; i < 4; i++ {
				wg.Add(1)
				go promoter()
			}
			wg.Wait()
			close(errCh)
			close(promotedCh)
			for err := range errCh {
				if err != nil {
					t.Fatalf("PromoteDueDelayed concurrent error = %v", err)
				}
			}
			promoted := 0
			for item := range promotedCh {
				promoted += item
			}
			if promoted != batch {
				t.Fatalf("promoted concurrently = %d, want %d", promoted, batch)
			}
			assertQueueLen(t, q.GetDelayedLength, 0, "delayed after concurrent promote")
			assertQueueLen(t, q.GetQueueLength, batch, "pending after concurrent promote")

			seen := make(map[uint]struct{}, batch)
			for i := 0; i < batch; i++ {
				msg, err := q.Dequeue(time.Second)
				if err != nil {
					t.Fatalf("Dequeue promoted message %d error = %v", i, err)
				}
				if _, exists := seen[msg.AccountID]; exists {
					t.Fatalf("duplicate promoted account id detected: %d", msg.AccountID)
				}
				seen[msg.AccountID] = struct{}{}
				if err := q.Ack(msg); err != nil {
					t.Fatalf("Ack promoted message %d error = %v", i, err)
				}
			}
			if len(seen) != batch {
				t.Fatalf("unique promoted messages = %d, want %d", len(seen), batch)
			}
			assertQueueLen(t, q.GetQueueLength, 0, "pending after promoted drain")
			assertQueueLen(t, q.GetProcessingLength, 0, "processing after promoted drain")
		})
	}
}

func TestTaskQueueRedisIntegrationRecoversOnlyStaleProcessing(t *testing.T) {
	if os.Getenv("CAIYUN_REDIS_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_REDIS_INTEGRATION=1 to run real Redis integration tests")
	}

	for _, tt := range integrationQueueBackends() {
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

			if err := q.Enqueue(9101, 601, "stale-first"); err != nil {
				t.Fatalf("Enqueue stale-first error = %v", err)
			}
			if err := q.Enqueue(9102, 601, "fresh-second"); err != nil {
				t.Fatalf("Enqueue fresh-second error = %v", err)
			}

			staleMsg, err := q.Dequeue(time.Second)
			if err != nil {
				t.Fatalf("Dequeue stale-first error = %v", err)
			}
			time.Sleep(1200 * time.Millisecond)

			freshMsg, err := q.Dequeue(time.Second)
			if err != nil {
				t.Fatalf("Dequeue fresh-second error = %v", err)
			}

			recovered, err := q.RecoverStaleProcessing(time.Second)
			if err != nil {
				t.Fatalf("RecoverStaleProcessing mixed error = %v", err)
			}
			if recovered != 1 {
				t.Fatalf("recovered = %d, want 1", recovered)
			}
			assertQueueLen(t, q.GetQueueLength, 1, "pending after mixed recovery")
			assertQueueLen(t, q.GetProcessingLength, 1, "processing after mixed recovery")

			if err := q.Ack(freshMsg); err != nil {
				t.Fatalf("Ack fresh-second error = %v", err)
			}
			assertQueueLen(t, q.GetProcessingLength, 0, "processing after fresh ack")

			restored, err := q.Dequeue(time.Second)
			if err != nil {
				t.Fatalf("Dequeue restored stale-first error = %v", err)
			}
			if restored.AccountID != staleMsg.AccountID || restored.UserID != staleMsg.UserID || restored.TaskType != staleMsg.TaskType {
				t.Fatalf("restored message = %+v, want stale payload %+v", restored, staleMsg)
			}
			if err := q.Ack(restored); err != nil {
				t.Fatalf("Ack restored stale-first error = %v", err)
			}
			assertQueueLen(t, q.GetQueueLength, 0, "pending after final ack")
			assertQueueLen(t, q.GetProcessingLength, 0, "processing after final ack")
		})
	}
}

func TestStreamTaskQueueRedisIntegrationRecoverAcrossConsumers(t *testing.T) {
	if os.Getenv("CAIYUN_REDIS_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_REDIS_INTEGRATION=1 to run real Redis integration tests")
	}

	redisCache := newIntegrationRedisCache(t)
	suffix := fmt.Sprintf("cross-consumer-%d", time.Now().UnixNano())
	baseOpts := StreamTaskQueueOptions{
		StreamKey:     fmt.Sprintf("%s:%s", TaskStreamKey, suffix),
		DelayedKey:    fmt.Sprintf("%s:%s", TaskStreamDelayedKey, suffix),
		DeadLetterKey: fmt.Sprintf("%s:%s", TaskStreamDeadLetterKey, suffix),
		ConsumerGroup: fmt.Sprintf("%s:%s", DefaultStreamConsumerGroup, suffix),
		ConsumerName:  "consumer-a",
	}
	consumerA := NewStreamTaskQueue(redisCache, baseOpts)
	consumerB := NewStreamTaskQueue(redisCache, StreamTaskQueueOptions{
		StreamKey:     baseOpts.StreamKey,
		DelayedKey:    baseOpts.DelayedKey,
		DeadLetterKey: baseOpts.DeadLetterKey,
		ConsumerGroup: baseOpts.ConsumerGroup,
		ConsumerName:  "consumer-b",
	})

	if err := consumerA.Clear(); err != nil {
		t.Fatalf("Clear() before test error = %v", err)
	}
	t.Cleanup(func() {
		_ = consumerA.Clear()
		_ = redisCache.Close()
	})

	if err := consumerA.Enqueue(8801, 501, "cross-consumer"); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	msg, err := consumerA.Dequeue(time.Second)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if msg == nil || msg.StreamID == "" {
		t.Fatalf("dequeued message should carry stream id: %+v", msg)
	}
	originalStreamID := msg.StreamID

	time.Sleep(1200 * time.Millisecond)
	recovered, err := consumerB.RecoverStaleProcessing(time.Second)
	if err != nil {
		t.Fatalf("RecoverStaleProcessing() error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}
	assertQueueLen(t, consumerB.GetQueueLength, 1, "pending after cross-consumer recovery")
	assertQueueLen(t, consumerB.GetProcessingLength, 0, "processing after cross-consumer recovery")

	restored, err := consumerB.Dequeue(time.Second)
	if err != nil {
		t.Fatalf("Dequeue() after recovery error = %v", err)
	}
	if restored.AccountID != 8801 || restored.UserID != 501 || restored.TaskType != "cross-consumer" {
		t.Fatalf("restored message = %+v, want original payload", restored)
	}
	if restored.StreamID == "" || restored.StreamID == originalStreamID {
		t.Fatalf("restored stream id = %q, want new stream id after re-enqueue (old=%q)", restored.StreamID, originalStreamID)
	}
	if err := consumerB.Ack(restored); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	assertQueueLen(t, consumerB.GetQueueLength, 0, "pending after final ack")
	assertQueueLen(t, consumerB.GetProcessingLength, 0, "processing after final ack")
}

type integrationQueueBackend struct {
	name  string
	build func(*cache.RedisCache) ReliableTaskQueue
}

func integrationQueueBackends() []integrationQueueBackend {
	return []integrationQueueBackend{
		{name: "list", build: func(redisCache *cache.RedisCache) ReliableTaskQueue {
			return NewTaskQueue(redisCache)
		}},
		{name: "streams", build: func(redisCache *cache.RedisCache) ReliableTaskQueue {
			return NewStreamTaskQueue(redisCache, StreamTaskQueueOptions{
				ConsumerName: fmt.Sprintf("integration-test-%d", time.Now().UnixNano()),
			})
		}},
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
