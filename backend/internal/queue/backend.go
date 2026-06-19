package queue

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/models"
)

var ErrQueueTimeout = errors.New("队列超时")

const (
	TaskQueueBackendList    = "list"
	TaskQueueBackendStreams = "streams"
)

// ReliableTaskQueue 是生产队列的稳定抽象。
//
// 当前支持 Redis List + processing/delayed/dead 辅助结构，以及 Redis Streams + consumer group。
type ReliableTaskQueue interface {
	Enqueue(accountID, userID uint, taskType string) error
	EnqueueBatch(accounts []*models.Account, taskType string) error
	Dequeue(timeout time.Duration) (*TaskMessage, error)
	Ack(message *TaskMessage) error
	Requeue(message *TaskMessage) error
	RequeueDelayed(message *TaskMessage, delay time.Duration) error
	DeadLetter(message *TaskMessage, reason string) error
	RecoverStaleProcessing(visibilityTimeout time.Duration) (int, error)
	PromoteDueDelayed(limit int64) (int, error)
	GetQueueLength() (int64, error)
	GetProcessingLength() (int64, error)
	GetDelayedLength() (int64, error)
	GetDeadLetterLength() (int64, error)
	Clear() error
}

// TaskQueueMetadata 暴露队列实现的低敏运行元数据，供状态接口、前端监控和灰度核验使用。
type TaskQueueMetadata struct {
	Backend       string            `json:"backend"`
	PendingKey    string            `json:"pending_key,omitempty"`
	ProcessingKey string            `json:"processing_key,omitempty"`
	DelayedKey    string            `json:"delayed_key,omitempty"`
	DeadLetterKey string            `json:"dead_letter_key,omitempty"`
	StreamKey     string            `json:"stream_key,omitempty"`
	ConsumerGroup string            `json:"consumer_group,omitempty"`
	ConsumerName  string            `json:"consumer_name,omitempty"`
	MaxLenApprox  int64             `json:"max_len_approx,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
}

// TaskQueueMetadataProvider 由具体队列实现提供运行元数据。
type TaskQueueMetadataProvider interface {
	Metadata() TaskQueueMetadata
}

var _ ReliableTaskQueue = (*TaskQueue)(nil)

// MetadataOf 返回队列元数据；未知实现也返回可序列化的 fallback。
func MetadataOf(taskQueue ReliableTaskQueue) TaskQueueMetadata {
	if taskQueue == nil {
		return TaskQueueMetadata{Backend: "none"}
	}
	if provider, ok := taskQueue.(TaskQueueMetadataProvider); ok {
		metadata := provider.Metadata()
		if metadata.Backend == "" {
			metadata.Backend = "unknown"
		}
		return metadata
	}
	return TaskQueueMetadata{Backend: "unknown"}
}

func NewConfiguredTaskQueue(redisCache *cache.RedisCache) (ReliableTaskQueue, error) {
	return NewTaskQueueBackend(redisCache, TaskQueueBackendFromEnv())
}

func NewTaskQueueBackend(redisCache *cache.RedisCache, backend string) (ReliableTaskQueue, error) {
	switch normalizeTaskQueueBackend(backend) {
	case TaskQueueBackendList:
		return NewTaskQueue(redisCache), nil
	case TaskQueueBackendStreams:
		return NewStreamTaskQueue(redisCache, StreamTaskQueueOptionsFromEnv()), nil
	default:
		return nil, fmt.Errorf("未知任务队列后端: %s", backend)
	}
}

func TaskQueueBackendFromEnv() string {
	return normalizeTaskQueueBackend(os.Getenv("TASK_QUEUE_BACKEND"))
}

func normalizeTaskQueueBackend(backend string) string {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "", TaskQueueBackendList:
		return TaskQueueBackendList
	case TaskQueueBackendStreams, "stream", "redis-stream", "redis-streams":
		return TaskQueueBackendStreams
	default:
		return strings.ToLower(strings.TrimSpace(backend))
	}
}

func envString(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}
