package queue

import (
	"fmt"
	"strings"
	"time"

	"caiyun/internal/envutil"
)

const (
	taskDedupeKeyPrefix  = "task:queue:dedupe:"
	defaultTaskDedupeTTL = 2 * time.Minute
)

type taskDedupeStore interface {
	SetNX(key string, value interface{}, expiration time.Duration) (bool, error)
}

type taskDedupeCleanupStore interface {
	ScanKeysByPrefix(prefix string, count int64) ([]string, error)
	Del(keys ...string) error
}

func taskDedupeEnabled() bool {
	return envutil.Bool("TASK_QUEUE_DEDUPE_ENABLED", true)
}

func taskDedupeTTL() time.Duration {
	return envutil.Duration("TASK_QUEUE_DEDUPE_TTL", defaultTaskDedupeTTL)
}

func taskDedupeKey(message *TaskMessage) string {
	if message == nil {
		return ""
	}
	// Operation outbox rows receive a new durable queue epoch whenever they are
	// retried or manually replayed. Prefer that explicit key over OperationID:
	// otherwise a still-live Redis dedupe claim would suppress a legitimate
	// failed -> queued transition for up to the dedupe TTL.
	if idempotencyKey := strings.TrimSpace(message.IdempotencyKey); idempotencyKey != "" {
		if strings.HasPrefix(idempotencyKey, taskDedupeKeyPrefix) {
			return idempotencyKey
		}
		return taskDedupeKeyPrefix + idempotencyKey
	}
	if operationID := strings.TrimSpace(message.OperationID); operationID != "" {
		return taskDedupeKeyPrefix + "operation:" + operationID
	}
	taskType := strings.TrimSpace(message.TaskType)
	if taskType == "" {
		taskType = "all"
	}
	return fmt.Sprintf("%s%d:%d:%s", taskDedupeKeyPrefix, message.UserID, message.AccountID, taskType)
}

type taskEnqueueDedupeClaim struct {
	Key     string
	Value   interface{}
	TTL     time.Duration
	Enabled bool
}

// prepareTaskEnqueueDedupe only derives the claim; it does not write Redis.
// Streams uses the returned claim in the same Lua script as XADD, while the
// legacy list queue continues to claim through SetNX.
func prepareTaskEnqueueDedupe(message *TaskMessage) taskEnqueueDedupeClaim {
	if !taskDedupeEnabled() || message == nil {
		return taskEnqueueDedupeClaim{}
	}
	ttl := taskDedupeTTL()
	if ttl <= 0 {
		return taskEnqueueDedupeClaim{}
	}
	key := taskDedupeKey(message)
	if key == "" {
		return taskEnqueueDedupeClaim{}
	}
	message.IdempotencyKey = key
	return taskEnqueueDedupeClaim{
		Key:     key,
		Value:   message.CreatedAt,
		TTL:     ttl,
		Enabled: true,
	}
}

func claimTaskEnqueueDedupe(store interface{}, message *TaskMessage) (bool, error) {
	claim := prepareTaskEnqueueDedupe(message)
	if !claim.Enabled {
		return true, nil
	}
	dedupeStore, ok := store.(taskDedupeStore)
	if !ok || dedupeStore == nil {
		// 非 Redis 测试替身或未来队列实现没有 SetNX 能力时，不强制去重。
		return true, nil
	}
	claimed, err := dedupeStore.SetNX(claim.Key, claim.Value, claim.TTL)
	if err != nil {
		return false, fmt.Errorf("写入任务幂等键失败: %w", err)
	}
	return claimed, nil
}

func clearTaskDedupeKeys(store interface{}) error {
	cleanup, ok := store.(taskDedupeCleanupStore)
	if !ok || cleanup == nil {
		return nil
	}
	keys, err := cleanup.ScanKeysByPrefix(taskDedupeKeyPrefix, 200)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return cleanup.Del(keys...)
}
