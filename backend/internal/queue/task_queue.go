package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/models"
)

const (
	TaskQueueKey           = "task:queue:pending"
	TaskQueueProcessingKey = "task:queue:processing"
)

// TaskQueue 任务队列
type TaskQueue struct {
	cache *cache.RedisCache
	ctx   context.Context
}

// TaskMessage 任务消息
type TaskMessage struct {
	AccountID  uint   `json:"account_id"`
	UserID     uint   `json:"user_id"`
	TaskType   string `json:"task_type"` // "all" 或具体任务类型
	CreatedAt  int64  `json:"created_at"`
	RetryCount int    `json:"retry_count"`
}

// NewTaskQueue 创建任务队列
func NewTaskQueue(redisCache *cache.RedisCache) *TaskQueue {
	return &TaskQueue{
		cache: redisCache,
		ctx:   context.Background(),
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
	// 使用BRPOP阻塞式弹出
	_, data, err := q.cache.BRPop(timeout, TaskQueueKey)
	if err != nil {
		return nil, err
	}

	var message TaskMessage
	if err := json.Unmarshal([]byte(data), &message); err != nil {
		return nil, fmt.Errorf("反序列化任务消息失败: %w", err)
	}

	return &message, nil
}

// GetQueueLength 获取队列长度
func (q *TaskQueue) GetQueueLength() (int64, error) {
	return q.cache.LLen(TaskQueueKey), nil
}

// Clear 清空队列
func (q *TaskQueue) Clear() error {
	return q.cache.Del(TaskQueueKey)
}
