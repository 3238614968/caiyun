package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/models"
)

const (
	TaskQueueKey           = "task:queue:pending"
	TaskProcessingKey      = "task:queue:processing"
	TaskDelayedKey         = "task:queue:delayed"
	TaskDeadLetterKey      = "task:queue:dead"
	DefaultMaxAttempts     = 3
	DefaultVisibilityDelay = 15 * time.Minute
	DefaultRetryBaseDelay  = 30 * time.Second
	DefaultRetryMaxDelay   = 5 * time.Minute
)

// TaskQueue 任务队列
type TaskQueue struct {
	cache redisQueueStore
	ctx   context.Context
}

type redisQueueStore interface {
	LPush(key string, values ...interface{}) error
	BRPopLPush(source, destination string, timeout time.Duration) (string, error)
	LRange(key string, start, stop int64) ([]string, error)
	LRem(key string, count int64, value interface{}) (int64, error)
	LLen(key string) int64
	ZAdd(key string, score float64, member interface{}) error
	ZRangeByScore(key string, min, max string, count int64) ([]string, error)
	ZRem(key string, members ...interface{}) (int64, error)
	ZCard(key string) int64
	Del(keys ...string) error
}

type atomicListQueueStore interface {
	ReplaceListItem(key, oldValue, newValue string) (bool, error)
	MoveListItemToList(source, destination, oldValue, newValue string) (bool, error)
	MoveListItemToZSet(source, destination, oldValue, newValue string, score float64) (bool, error)
	MoveZSetItemToList(source, destination, member string) (bool, error)
}

// TaskMessage 任务消息
type TaskMessage struct {
	AccountID      uint   `json:"account_id"`
	UserID         uint   `json:"user_id"`
	TaskType       string `json:"task_type"` // "all" 或具体任务类型
	OperationID    string `json:"operation_id,omitempty"`
	OperationType  string `json:"operation_type,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	RetryCount     int    `json:"retry_count"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`

	// ProcessingAt 仅用于可靠队列的可见性超时恢复。
	ProcessingAt int64 `json:"processing_at,omitempty"`

	// StreamID 仅用于 Redis Streams 后端的 XACK/XAUTOCLAIM。
	StreamID string `json:"-"`

	raw string
}

// NewTaskQueue 创建任务队列
func NewTaskQueue(redisCache *cache.RedisCache) *TaskQueue {
	return newTaskQueueWithStore(redisCache)
}

func newTaskQueueWithStore(store redisQueueStore) *TaskQueue {
	return &TaskQueue{
		cache: store,
		ctx:   context.Background(),
	}
}

// Metadata 返回 Redis List 队列的运行元数据。
func (q *TaskQueue) Metadata() TaskQueueMetadata {
	return TaskQueueMetadata{
		Backend:       TaskQueueBackendList,
		PendingKey:    TaskQueueKey,
		ProcessingKey: TaskProcessingKey,
		DelayedKey:    TaskDelayedKey,
		DeadLetterKey: TaskDeadLetterKey,
	}
}

// Enqueue 将任务加入队列
func (q *TaskQueue) Enqueue(accountID, userID uint, taskType string) error {
	message := TaskMessage{
		AccountID:  accountID,
		UserID:     userID,
		TaskType:   taskType,
		CreatedAt:  time.Now().Unix(),
		RetryCount: 0,
	}
	return q.EnqueueMessage(&message)
}

// EnqueueMessage publishes an already constructed command. It is used by the
// durable operation outbox while Enqueue remains the compatibility API for
// scheduled account tasks.
func (q *TaskQueue) EnqueueMessage(message *TaskMessage) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if message.CreatedAt == 0 {
		message.CreatedAt = time.Now().Unix()
	}

	claimed, err := claimTaskEnqueueDedupe(q.cache, message)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化任务消息失败: %w", err)
	}

	// 使用Redis List的LPUSH添加到队列头部
	if err := q.cache.LPush(TaskQueueKey, string(data)); err != nil {
		return fmt.Errorf("加入队列失败: %w", err)
	}

	return nil
}

// Requeue 将失败任务按指数退避放入延迟队列，避免失败后立即重试打爆上游接口。
func (q *TaskQueue) Requeue(message *TaskMessage) error {
	return q.RequeueDelayed(message, retryBackoff(message))
}

// RenewVisibility refreshes the processing timestamp for a List-backed
// delivery.  The timestamp is part of the exact list value, so replace it
// atomically when the cache supports the queue Lua primitives.  A missing
// value is not an error: another recovery path may already have moved it.
func (q *TaskQueue) RenewVisibility(message *TaskMessage) (bool, error) {
	if message == nil {
		return false, fmt.Errorf("任务消息为空")
	}
	raw, err := messageRaw(message)
	if err != nil {
		return false, err
	}

	updated := *message
	updated.ProcessingAt = time.Now().Unix()
	updated.raw = ""
	payloadBytes, err := json.Marshal(&updated)
	if err != nil {
		return false, fmt.Errorf("序列化续约任务失败: %w", err)
	}
	payload := string(payloadBytes)
	if payload == raw {
		return true, nil
	}

	if atomic, ok := q.cache.(atomicListQueueStore); ok {
		renewed, err := atomic.ReplaceListItem(TaskProcessingKey, raw, payload)
		if err != nil {
			return false, fmt.Errorf("续约处理中任务失败: %w", err)
		}
		if renewed {
			message.ProcessingAt = updated.ProcessingAt
			message.raw = payload
		}
		return renewed, nil
	}

	removed, err := q.cache.LRem(TaskProcessingKey, 1, raw)
	if err != nil {
		return false, fmt.Errorf("续约处理中任务失败: %w", err)
	}
	if removed == 0 {
		return false, nil
	}
	if err := q.cache.LPush(TaskProcessingKey, payload); err != nil {
		// Keep the old representation recoverable if the replacement write
		// fails after removal.  The original error remains the actionable one.
		_ = q.cache.LPush(TaskProcessingKey, raw)
		return false, fmt.Errorf("写入续约处理中任务失败: %w", err)
	}
	message.ProcessingAt = updated.ProcessingAt
	message.raw = payload
	return true, nil
}

// RequeueDelayed 将失败任务移入延迟队列，到期后由 Worker 维护循环恢复到 pending。
func (q *TaskQueue) RequeueDelayed(message *TaskMessage, delay time.Duration) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	raw, err := messageRaw(message)
	if err != nil {
		return err
	}
	message.ProcessingAt = 0
	if delay < 0 {
		delay = 0
	}

	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("序列化任务消息失败: %w", err)
	}
	payload := string(data)
	if delay == 0 {
		if atomic, ok := q.cache.(atomicListQueueStore); ok {
			moved, err := atomic.MoveListItemToList(TaskProcessingKey, TaskQueueKey, raw, payload)
			if err != nil {
				return fmt.Errorf("重新加入队列失败: %w", err)
			}
			if !moved {
				return fmt.Errorf("重新加入队列失败: 原始处理中消息不存在")
			}
			return nil
		}
		if err := q.removeProcessingRaw(raw); err != nil {
			return err
		}
		if err := q.cache.LPush(TaskQueueKey, payload); err != nil {
			return fmt.Errorf("重新加入队列失败: %w", err)
		}
		return nil
	}

	availableAt := time.Now().Add(delay).Unix()
	if atomic, ok := q.cache.(atomicListQueueStore); ok {
		moved, err := atomic.MoveListItemToZSet(TaskProcessingKey, TaskDelayedKey, raw, payload, float64(availableAt))
		if err != nil {
			return fmt.Errorf("加入延迟队列失败: %w", err)
		}
		if !moved {
			return fmt.Errorf("加入延迟队列失败: 原始处理中消息不存在")
		}
		return nil
	}
	if err := q.removeProcessingRaw(raw); err != nil {
		return err
	}
	if err := q.cache.ZAdd(TaskDelayedKey, float64(availableAt), payload); err != nil {
		return fmt.Errorf("加入延迟队列失败: %w", err)
	}
	return nil
}

// DeadLetter 将超过重试次数的任务移入死信队列，便于后续人工排查或重放。
func (q *TaskQueue) DeadLetter(message *TaskMessage, reason string) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	raw, err := messageRaw(message)
	if err != nil {
		return err
	}
	message.ProcessingAt = 0
	deadPayload, err := encodeListDeadLetter(message, "", reason)
	if err != nil {
		return fmt.Errorf("序列化死信消息失败: %w", err)
	}
	if atomic, ok := q.cache.(atomicListQueueStore); ok {
		moved, err := atomic.MoveListItemToList(TaskProcessingKey, TaskDeadLetterKey, raw, deadPayload)
		if err != nil {
			return fmt.Errorf("写入死信队列失败: %w", err)
		}
		if !moved {
			return fmt.Errorf("写入死信队列失败: 原始处理中消息不存在")
		}
		return nil
	}
	if err := q.removeProcessingRaw(raw); err != nil {
		return err
	}
	if err := q.cache.LPush(TaskDeadLetterKey, deadPayload); err != nil {
		return fmt.Errorf("写入死信队列失败: %w", err)
	}
	return nil
}

// EnqueueBatch 批量加入队列
func (q *TaskQueue) EnqueueBatch(accounts []*models.Account, taskType string) error {
	for _, account := range accounts {
		if err := q.Enqueue(account.ID, account.UserID, taskType); err != nil {
			return fmt.Errorf("批量加入队列失败: %w", err)
		}
	}
	return nil
}

// Dequeue 从队列取出任务（阻塞）
func (q *TaskQueue) Dequeue(timeout time.Duration) (*TaskMessage, error) {
	// 使用 BRPOPLPUSH 原子地把任务从 pending 移到 processing。
	// 后续成功 Ack、失败 Requeue/DeadLetter，避免 Worker 崩溃时任务直接丢失。
	data, err := q.cache.BRPopLPush(TaskQueueKey, TaskProcessingKey, timeout)
	if err != nil {
		if strings.Contains(err.Error(), "队列超时") {
			return nil, ErrQueueTimeout
		}
		return nil, err
	}

	var message TaskMessage
	if err := json.Unmarshal([]byte(data), &message); err != nil {
		if dlqErr := q.deadLetterRaw(data, fmt.Sprintf("反序列化任务消息失败: %v", err)); dlqErr != nil {
			return nil, fmt.Errorf("反序列化任务消息失败: %w；写入死信失败: %v", err, dlqErr)
		}
		return nil, fmt.Errorf("反序列化任务消息失败: %w", err)
	}
	message.ProcessingAt = time.Now().Unix()

	updated, err := json.Marshal(message)
	if err != nil {
		return nil, fmt.Errorf("序列化任务消息失败: %w", err)
	}
	updatedData := string(updated)
	if updatedData != data {
		if atomic, ok := q.cache.(atomicListQueueStore); ok {
			replaced, err := atomic.ReplaceListItem(TaskProcessingKey, data, updatedData)
			if err != nil {
				return nil, fmt.Errorf("更新处理中任务失败: %w", err)
			}
			if !replaced {
				return nil, fmt.Errorf("更新处理中任务失败: 原始消息不存在")
			}
		} else {
			removed, err := q.cache.LRem(TaskProcessingKey, 1, data)
			if err != nil {
				return nil, fmt.Errorf("更新处理中任务失败: %w", err)
			}
			if removed == 0 {
				return nil, fmt.Errorf("更新处理中任务失败: 原始消息不存在")
			}
			if err := q.cache.LPush(TaskProcessingKey, updatedData); err != nil {
				_ = q.cache.LPush(TaskProcessingKey, data)
				return nil, fmt.Errorf("写入处理中任务失败: %w", err)
			}
		}
		data = updatedData
	}
	message.raw = data

	return &message, nil
}

func (q *TaskQueue) deadLetterRaw(raw, reason string) error {
	deadPayload, err := encodeListDeadLetter(nil, raw, reason)
	if err != nil {
		return fmt.Errorf("序列化死信消息失败: %w", err)
	}
	if atomic, ok := q.cache.(atomicListQueueStore); ok {
		moved, err := atomic.MoveListItemToList(TaskProcessingKey, TaskDeadLetterKey, raw, deadPayload)
		if err != nil {
			return fmt.Errorf("写入死信队列失败: %w", err)
		}
		if !moved {
			return fmt.Errorf("写入死信队列失败: 原始处理中消息不存在")
		}
		return nil
	}
	removed, err := q.cache.LRem(TaskProcessingKey, 1, raw)
	if err != nil {
		return fmt.Errorf("写入死信队列失败: %w", err)
	}
	if removed == 0 {
		return fmt.Errorf("写入死信队列失败: 原始处理中消息不存在")
	}
	if err := q.cache.LPush(TaskDeadLetterKey, deadPayload); err != nil {
		return fmt.Errorf("写入死信队列失败: %w", err)
	}
	return nil
}

// Ack 确认任务已成功处理，从 processing 队列移除。
func (q *TaskQueue) Ack(message *TaskMessage) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	return q.removeProcessing(message)
}

// RecoverStaleProcessing 将超过可见性超时时间的处理中任务恢复到 pending 队列。
func (q *TaskQueue) RecoverStaleProcessing(visibilityTimeout time.Duration) (int, error) {
	if visibilityTimeout <= 0 {
		visibilityTimeout = DefaultVisibilityDelay
	}

	items, err := q.cache.LRange(TaskProcessingKey, 0, -1)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-visibilityTimeout).Unix()
	recovered := 0
	for _, item := range items {
		var message TaskMessage
		if err := json.Unmarshal([]byte(item), &message); err != nil {
			continue
		}
		processingAt := message.ProcessingAt
		if processingAt == 0 {
			processingAt = message.CreatedAt
		}
		if processingAt == 0 || processingAt > cutoff {
			continue
		}

		message.ProcessingAt = 0
		data, err := json.Marshal(message)
		if err != nil {
			return recovered, err
		}
		payload := string(data)
		if atomic, ok := q.cache.(atomicListQueueStore); ok {
			moved, err := atomic.MoveListItemToList(TaskProcessingKey, TaskQueueKey, item, payload)
			if err != nil {
				return recovered, err
			}
			if !moved {
				continue
			}
		} else {
			if _, err := q.cache.LRem(TaskProcessingKey, 1, item); err != nil {
				return recovered, err
			}
			if err := q.cache.LPush(TaskQueueKey, payload); err != nil {
				return recovered, err
			}
		}
		recovered++
	}

	return recovered, nil
}

// PromoteDueDelayed 将到期的延迟任务恢复到 pending 队列。
func (q *TaskQueue) PromoteDueDelayed(limit int64) (int, error) {
	if limit <= 0 {
		limit = 100
	}

	items, err := q.cache.ZRangeByScore(TaskDelayedKey, "-inf", fmt.Sprintf("%d", time.Now().Unix()), limit)
	if err != nil {
		return 0, err
	}

	promoted := 0
	for _, item := range items {
		if atomic, ok := q.cache.(atomicListQueueStore); ok {
			moved, err := atomic.MoveZSetItemToList(TaskDelayedKey, TaskQueueKey, item)
			if err != nil {
				return promoted, err
			}
			if !moved {
				continue
			}
			promoted++
			continue
		}
		removed, err := q.cache.ZRem(TaskDelayedKey, item)
		if err != nil {
			return promoted, err
		}
		if removed == 0 {
			continue
		}
		if err := q.cache.LPush(TaskQueueKey, item); err != nil {
			_ = q.cache.ZAdd(TaskDelayedKey, float64(time.Now().Add(DefaultRetryBaseDelay).Unix()), item)
			return promoted, err
		}
		promoted++
	}

	return promoted, nil
}

// GetQueueLength 获取队列长度
func (q *TaskQueue) GetQueueLength() (int64, error) {
	return q.cache.LLen(TaskQueueKey), nil
}

func (q *TaskQueue) GetDeadLetterLength() (int64, error) {
	return q.cache.LLen(TaskDeadLetterKey), nil
}

// ListDeadLetters returns the newest dead-letter entries up to limit.  It is
// an inspection API; only ReplayDeadLetter performs a state transition.
func (q *TaskQueue) ListDeadLetters(limit int) ([]*DeadLetterMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rawItems, err := q.cache.LRange(TaskDeadLetterKey, 0, int64(limit-1))
	if err != nil {
		return nil, err
	}
	items := make([]*DeadLetterMessage, 0, len(rawItems))
	for _, raw := range rawItems {
		item, _, parseErr := parseListDeadLetter(raw)
		if parseErr != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (q *TaskQueue) GetDeadLetter(id string) (*DeadLetterMessage, error) {
	items, err := q.listDeadLettersWithRaw()
	if err != nil {
		return nil, err
	}
	for _, entry := range items {
		if entry.item.ID == id {
			return entry.item, nil
		}
	}
	return nil, ErrDeadLetterNotFound
}

// ReplayDeadLetter atomically moves a non-Operation message back to pending
// whenever the store provides list move primitives. Operation messages are
// replayed through OperationService so their durable state is transitioned
// before Redis delivery is retried.
func (q *TaskQueue) ReplayDeadLetter(id string) (*TaskMessage, error) {
	entries, err := q.listDeadLettersWithRaw()
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.item.ID != id {
			continue
		}
		if entry.item.Malformed || entry.task == nil {
			return nil, fmt.Errorf("malformed dead-letter entry cannot be replayed")
		}
		if entry.task.OperationID != "" {
			return nil, ErrOperationDeadLetterReplay
		}
		message := resetReplayMessage(entry.task)
		payload, err := json.Marshal(message)
		if err != nil {
			return nil, err
		}
		if atomic, ok := q.cache.(atomicListQueueStore); ok {
			moved, err := atomic.MoveListItemToList(TaskDeadLetterKey, TaskQueueKey, entry.raw, string(payload))
			if err != nil {
				return nil, err
			}
			if !moved {
				return nil, ErrDeadLetterNotFound
			}
			return message, nil
		}
		removed, err := q.cache.LRem(TaskDeadLetterKey, 1, entry.raw)
		if err != nil {
			return nil, err
		}
		if removed != 1 {
			return nil, ErrDeadLetterNotFound
		}
		if err := q.cache.LPush(TaskQueueKey, string(payload)); err != nil {
			_ = q.cache.LPush(TaskDeadLetterKey, entry.raw)
			return nil, err
		}
		return message, nil
	}
	return nil, ErrDeadLetterNotFound
}

// ArchiveDeadLetter removes an entry after OperationService has durably
// re-queued the associated command. It deliberately does not enqueue again.
func (q *TaskQueue) ArchiveDeadLetter(id string) error {
	entries, err := q.listDeadLettersWithRaw()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.item.ID != id {
			continue
		}
		removed, err := q.cache.LRem(TaskDeadLetterKey, 1, entry.raw)
		if err != nil {
			return err
		}
		if removed != 1 {
			return ErrDeadLetterNotFound
		}
		return nil
	}
	return ErrDeadLetterNotFound
}

type listDeadLetterRaw struct {
	raw  string
	item *DeadLetterMessage
	task *TaskMessage
}

func (q *TaskQueue) listDeadLettersWithRaw() ([]listDeadLetterRaw, error) {
	rawItems, err := q.cache.LRange(TaskDeadLetterKey, 0, -1)
	if err != nil {
		return nil, err
	}
	items := make([]listDeadLetterRaw, 0, len(rawItems))
	for _, raw := range rawItems {
		item, task, parseErr := parseListDeadLetter(raw)
		if parseErr != nil {
			continue
		}
		items = append(items, listDeadLetterRaw{raw: raw, item: item, task: task})
	}
	return items, nil
}

func (q *TaskQueue) GetProcessingLength() (int64, error) {
	return q.cache.LLen(TaskProcessingKey), nil
}

func (q *TaskQueue) GetDelayedLength() (int64, error) {
	return q.cache.ZCard(TaskDelayedKey), nil
}

// Clear 清空队列
func (q *TaskQueue) Clear() error {
	if err := q.cache.Del(TaskQueueKey, TaskProcessingKey, TaskDelayedKey, TaskDeadLetterKey); err != nil {
		return err
	}
	return clearTaskDedupeKeys(q.cache)
}

func (q *TaskQueue) removeProcessing(message *TaskMessage) error {
	raw, err := messageRaw(message)
	if err != nil {
		return err
	}
	return q.removeProcessingRaw(raw)
}

func (q *TaskQueue) removeProcessingRaw(raw string) error {
	if _, err := q.cache.LRem(TaskProcessingKey, 1, raw); err != nil {
		return fmt.Errorf("移除处理中任务失败: %w", err)
	}
	return nil
}

func messageRaw(message *TaskMessage) (string, error) {
	if message == nil {
		return "", fmt.Errorf("任务消息为空")
	}
	if message.raw != "" {
		return message.raw, nil
	}
	data, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("序列化任务消息失败: %w", err)
	}
	return string(data), nil
}

func retryBackoff(message *TaskMessage) time.Duration {
	if message == nil || message.RetryCount <= 0 {
		return DefaultRetryBaseDelay
	}

	delay := DefaultRetryBaseDelay
	for i := 1; i < message.RetryCount; i++ {
		delay *= 2
		if delay >= DefaultRetryMaxDelay {
			return DefaultRetryMaxDelay
		}
	}
	return delay
}
