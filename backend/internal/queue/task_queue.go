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

// TaskMessage 任务消息
type TaskMessage struct {
	AccountID  uint   `json:"account_id"`
	UserID     uint   `json:"user_id"`
	TaskType   string `json:"task_type"` // "all" 或具体任务类型
	CreatedAt  int64  `json:"created_at"`
	RetryCount int    `json:"retry_count"`

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

// RequeueDelayed 将失败任务移入延迟队列，到期后由 Worker 维护循环恢复到 pending。
func (q *TaskQueue) RequeueDelayed(message *TaskMessage, delay time.Duration) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if err := q.removeProcessing(message); err != nil {
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
	if delay == 0 {
		if err := q.cache.LPush(TaskQueueKey, string(data)); err != nil {
			return fmt.Errorf("重新加入队列失败: %w", err)
		}
		return nil
	}

	availableAt := time.Now().Add(delay).Unix()
	if err := q.cache.ZAdd(TaskDelayedKey, float64(availableAt), string(data)); err != nil {
		return fmt.Errorf("加入延迟队列失败: %w", err)
	}
	return nil
}

// DeadLetter 将超过重试次数的任务移入死信队列，便于后续人工排查或重放。
func (q *TaskQueue) DeadLetter(message *TaskMessage, reason string) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if err := q.removeProcessing(message); err != nil {
		return err
	}
	message.ProcessingAt = 0
	payload := map[string]interface{}{
		"message":   message,
		"reason":    reason,
		"failed_at": time.Now().Unix(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化死信消息失败: %w", err)
	}
	if err := q.cache.LPush(TaskDeadLetterKey, string(data)); err != nil {
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
		data = updatedData
	}
	message.raw = data

	return &message, nil
}

func (q *TaskQueue) deadLetterRaw(raw, reason string) error {
	_, _ = q.cache.LRem(TaskProcessingKey, 1, raw)
	payload := map[string]interface{}{
		"raw_message": raw,
		"reason":      reason,
		"failed_at":   time.Now().Unix(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化死信消息失败: %w", err)
	}
	if err := q.cache.LPush(TaskDeadLetterKey, string(data)); err != nil {
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

		if _, err := q.cache.LRem(TaskProcessingKey, 1, item); err != nil {
			return recovered, err
		}
		message.ProcessingAt = 0
		data, err := json.Marshal(message)
		if err != nil {
			return recovered, err
		}
		if err := q.cache.LPush(TaskQueueKey, string(data)); err != nil {
			return recovered, err
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

func (q *TaskQueue) GetProcessingLength() (int64, error) {
	return q.cache.LLen(TaskProcessingKey), nil
}

func (q *TaskQueue) GetDelayedLength() (int64, error) {
	return q.cache.ZCard(TaskDelayedKey), nil
}

// Clear 清空队列
func (q *TaskQueue) Clear() error {
	return q.cache.Del(TaskQueueKey, TaskProcessingKey, TaskDelayedKey, TaskDeadLetterKey)
}

func (q *TaskQueue) removeProcessing(message *TaskMessage) error {
	raw := message.raw
	if raw == "" {
		data, err := json.Marshal(message)
		if err != nil {
			return fmt.Errorf("序列化任务消息失败: %w", err)
		}
		raw = string(data)
	}

	if _, err := q.cache.LRem(TaskProcessingKey, 1, raw); err != nil {
		return fmt.Errorf("移除处理中任务失败: %w", err)
	}
	return nil
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
