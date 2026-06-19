package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/models"
)

const (
	TaskStreamKey               = "task:queue:stream"
	TaskStreamDelayedKey        = "task:queue:stream:delayed"
	TaskStreamDeadLetterKey     = "task:queue:stream:dead"
	DefaultStreamConsumerGroup  = "caiyun-workers"
	DefaultStreamMaxLenApprox   = 100000
	defaultStreamReadCount      = 1
	defaultStreamRecoverBatch   = 100
	streamPayloadField          = "payload"
	streamDeadReasonField       = "reason"
	streamDeadFailedAtField     = "failed_at"
	streamDeadOriginalIDField   = "original_id"
	streamDeadOriginalDataField = "original_payload"
	streamConsumerNameEnv       = "TASK_QUEUE_STREAM_CONSUMER"
	streamConsumerGroupEnv      = "TASK_QUEUE_STREAM_GROUP"
	streamKeyEnv                = "TASK_QUEUE_STREAM_KEY"
	streamDelayedKeyEnv         = "TASK_QUEUE_STREAM_DELAYED_KEY"
	streamDeadKeyEnv            = "TASK_QUEUE_STREAM_DEAD_KEY"
	streamMaxLenEnv             = "TASK_QUEUE_STREAM_MAXLEN"
)

type streamQueueStore interface {
	XGroupCreateMkStream(stream, group, start string) error
	XAdd(stream string, maxLenApprox int64, values map[string]interface{}) (string, error)
	XReadGroup(group, consumer, stream, id string, count int64, block time.Duration) ([]cache.StreamMessage, error)
	XAck(stream, group string, ids ...string) (int64, error)
	XDel(stream string, ids ...string) (int64, error)
	XAutoClaim(stream, group, consumer string, minIdle time.Duration, start string, count int64) ([]cache.StreamMessage, string, error)
	XPendingCount(stream, group string) (int64, error)
	XLen(stream string) int64
	ZAdd(key string, score float64, member interface{}) error
	ZRangeByScore(key string, min, max string, count int64) ([]string, error)
	ZRem(key string, members ...interface{}) (int64, error)
	ZCard(key string) int64
	Del(keys ...string) error
}

type StreamTaskQueueOptions struct {
	StreamKey     string
	DelayedKey    string
	DeadLetterKey string
	ConsumerGroup string
	ConsumerName  string
	MaxLenApprox  int64
}

type StreamTaskQueue struct {
	cache streamQueueStore
	opts  StreamTaskQueueOptions
}

var _ ReliableTaskQueue = (*StreamTaskQueue)(nil)

func StreamTaskQueueOptionsFromEnv() StreamTaskQueueOptions {
	return StreamTaskQueueOptions{
		StreamKey:     envString(streamKeyEnv, TaskStreamKey),
		DelayedKey:    envString(streamDelayedKeyEnv, TaskStreamDelayedKey),
		DeadLetterKey: envString(streamDeadKeyEnv, TaskStreamDeadLetterKey),
		ConsumerGroup: envString(streamConsumerGroupEnv, DefaultStreamConsumerGroup),
		ConsumerName:  envString(streamConsumerNameEnv, defaultStreamConsumerName()),
		MaxLenApprox:  envInt64(streamMaxLenEnv, DefaultStreamMaxLenApprox),
	}
}

func NewStreamTaskQueue(redisCache *cache.RedisCache, opts StreamTaskQueueOptions) *StreamTaskQueue {
	return newStreamTaskQueueWithStore(redisCache, opts)
}

func newStreamTaskQueueWithStore(store streamQueueStore, opts StreamTaskQueueOptions) *StreamTaskQueue {
	opts = normalizeStreamOptions(opts)
	q := &StreamTaskQueue{cache: store, opts: opts}
	return q
}

// Metadata 返回 Redis Streams 队列的运行元数据。
func (q *StreamTaskQueue) Metadata() TaskQueueMetadata {
	return TaskQueueMetadata{
		Backend:       TaskQueueBackendStreams,
		StreamKey:     q.opts.StreamKey,
		DelayedKey:    q.opts.DelayedKey,
		DeadLetterKey: q.opts.DeadLetterKey,
		ConsumerGroup: q.opts.ConsumerGroup,
		ConsumerName:  q.opts.ConsumerName,
		MaxLenApprox:  q.opts.MaxLenApprox,
		Labels: map[string]string{
			"delivery": "consumer-group",
			"ack":      "xack+xdel",
		},
	}
}

func normalizeStreamOptions(opts StreamTaskQueueOptions) StreamTaskQueueOptions {
	if opts.StreamKey == "" {
		opts.StreamKey = TaskStreamKey
	}
	if opts.DelayedKey == "" {
		opts.DelayedKey = TaskStreamDelayedKey
	}
	if opts.DeadLetterKey == "" {
		opts.DeadLetterKey = TaskStreamDeadLetterKey
	}
	if opts.ConsumerGroup == "" {
		opts.ConsumerGroup = DefaultStreamConsumerGroup
	}
	if opts.ConsumerName == "" {
		opts.ConsumerName = defaultStreamConsumerName()
	}
	if opts.MaxLenApprox <= 0 {
		opts.MaxLenApprox = DefaultStreamMaxLenApprox
	}
	return opts
}

func defaultStreamConsumerName() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "worker"
	}
	return fmt.Sprintf("%s:%d", hostname, os.Getpid())
}

func (q *StreamTaskQueue) ensureGroup() error {
	return q.cache.XGroupCreateMkStream(q.opts.StreamKey, q.opts.ConsumerGroup, "0")
}

func (q *StreamTaskQueue) Enqueue(accountID, userID uint, taskType string) error {
	message := TaskMessage{
		AccountID:  accountID,
		UserID:     userID,
		TaskType:   taskType,
		CreatedAt:  time.Now().Unix(),
		RetryCount: 0,
	}
	return q.enqueueMessage(&message)
}

func (q *StreamTaskQueue) EnqueueBatch(accounts []*models.Account, taskType string) error {
	for _, account := range accounts {
		if err := q.Enqueue(account.ID, account.UserID, taskType); err != nil {
			return fmt.Errorf("批量加入 Streams 队列失败: %w", err)
		}
	}
	return nil
}

func (q *StreamTaskQueue) Dequeue(timeout time.Duration) (*TaskMessage, error) {
	if err := q.ensureGroup(); err != nil {
		return nil, err
	}
	messages, err := q.cache.XReadGroup(
		q.opts.ConsumerGroup,
		q.opts.ConsumerName,
		q.opts.StreamKey,
		">",
		defaultStreamReadCount,
		timeout,
	)
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		return nil, ErrQueueTimeout
	}
	return decodeStreamMessage(messages[0])
}

func (q *StreamTaskQueue) Ack(message *TaskMessage) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if message.StreamID == "" {
		return fmt.Errorf("Streams 消息缺少 StreamID")
	}
	if _, err := q.cache.XAck(q.opts.StreamKey, q.opts.ConsumerGroup, message.StreamID); err != nil {
		return fmt.Errorf("确认 Streams 任务失败: %w", err)
	}
	if _, err := q.cache.XDel(q.opts.StreamKey, message.StreamID); err != nil {
		return fmt.Errorf("删除已确认 Streams 任务失败: %w", err)
	}
	return nil
}

func (q *StreamTaskQueue) Requeue(message *TaskMessage) error {
	return q.RequeueDelayed(message, retryBackoff(message))
}

func (q *StreamTaskQueue) RequeueDelayed(message *TaskMessage, delay time.Duration) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if err := q.Ack(message); err != nil {
		return err
	}

	message.StreamID = ""
	message.ProcessingAt = 0
	if delay < 0 {
		delay = 0
	}
	if delay == 0 {
		return q.enqueueMessage(message)
	}

	payload, err := encodeTaskMessage(message)
	if err != nil {
		return err
	}
	availableAt := time.Now().Add(delay).Unix()
	if err := q.cache.ZAdd(q.opts.DelayedKey, float64(availableAt), payload); err != nil {
		return fmt.Errorf("加入 Streams 延迟队列失败: %w", err)
	}
	return nil
}

func (q *StreamTaskQueue) DeadLetter(message *TaskMessage, reason string) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}

	payload, err := encodeTaskMessage(message)
	if err != nil {
		return err
	}
	if _, err := q.cache.XAdd(q.opts.DeadLetterKey, q.opts.MaxLenApprox, map[string]interface{}{
		streamDeadOriginalIDField:   message.StreamID,
		streamDeadReasonField:       reason,
		streamDeadFailedAtField:     time.Now().Unix(),
		streamDeadOriginalDataField: payload,
	}); err != nil {
		return fmt.Errorf("写入 Streams 死信队列失败: %w", err)
	}
	return q.Ack(message)
}

func (q *StreamTaskQueue) RecoverStaleProcessing(visibilityTimeout time.Duration) (int, error) {
	if visibilityTimeout <= 0 {
		visibilityTimeout = DefaultVisibilityDelay
	}
	if err := q.ensureGroup(); err != nil {
		return 0, err
	}

	messages, _, err := q.cache.XAutoClaim(
		q.opts.StreamKey,
		q.opts.ConsumerGroup,
		q.opts.ConsumerName,
		visibilityTimeout,
		"0-0",
		defaultStreamRecoverBatch,
	)
	if err != nil {
		return 0, err
	}

	recovered := 0
	for _, streamMessage := range messages {
		message, err := decodeStreamMessage(streamMessage)
		if err != nil {
			continue
		}
		if _, err := q.cache.XAck(q.opts.StreamKey, q.opts.ConsumerGroup, message.StreamID); err != nil {
			return recovered, err
		}
		if _, err := q.cache.XDel(q.opts.StreamKey, message.StreamID); err != nil {
			return recovered, err
		}
		message.StreamID = ""
		message.ProcessingAt = 0
		if err := q.enqueueMessage(message); err != nil {
			return recovered, err
		}
		recovered++
	}

	return recovered, nil
}

func (q *StreamTaskQueue) PromoteDueDelayed(limit int64) (int, error) {
	if limit <= 0 {
		limit = 100
	}

	items, err := q.cache.ZRangeByScore(q.opts.DelayedKey, "-inf", fmt.Sprintf("%d", time.Now().Unix()), limit)
	if err != nil {
		return 0, err
	}

	promoted := 0
	for _, item := range items {
		removed, err := q.cache.ZRem(q.opts.DelayedKey, item)
		if err != nil {
			return promoted, err
		}
		if removed == 0 {
			continue
		}
		if err := q.enqueuePayload(item); err != nil {
			_ = q.cache.ZAdd(q.opts.DelayedKey, float64(time.Now().Add(DefaultRetryBaseDelay).Unix()), item)
			return promoted, err
		}
		promoted++
	}

	return promoted, nil
}

func (q *StreamTaskQueue) GetQueueLength() (int64, error) {
	pending, err := q.GetProcessingLength()
	if err != nil {
		return 0, err
	}
	// Redis Streams 不提供按 consumer group 精确统计“可立即消费消息数”的单条命令。
	// 本队列在 Ack 后会 XDEL 已完成消息，因此 XLen - Pending 可作为监控面板的近似值；
	// 在高并发读写瞬间可能存在轻微竞态误差，业务可靠性以 XACK/XAUTOCLAIM 生命周期为准。
	length := q.cache.XLen(q.opts.StreamKey) - pending
	if length < 0 {
		return 0, nil
	}
	return length, nil
}

func (q *StreamTaskQueue) GetProcessingLength() (int64, error) {
	if err := q.ensureGroup(); err != nil {
		return 0, err
	}
	return q.cache.XPendingCount(q.opts.StreamKey, q.opts.ConsumerGroup)
}

func (q *StreamTaskQueue) GetDelayedLength() (int64, error) {
	return q.cache.ZCard(q.opts.DelayedKey), nil
}

func (q *StreamTaskQueue) GetDeadLetterLength() (int64, error) {
	return q.cache.XLen(q.opts.DeadLetterKey), nil
}

func (q *StreamTaskQueue) Clear() error {
	if err := q.cache.Del(q.opts.StreamKey, q.opts.DelayedKey, q.opts.DeadLetterKey); err != nil {
		return err
	}
	return q.ensureGroup()
}

func (q *StreamTaskQueue) enqueueMessage(message *TaskMessage) error {
	payload, err := encodeTaskMessage(message)
	if err != nil {
		return err
	}
	return q.enqueuePayload(payload)
}

func (q *StreamTaskQueue) enqueuePayload(payload string) error {
	if err := q.ensureGroup(); err != nil {
		return err
	}
	if _, err := q.cache.XAdd(q.opts.StreamKey, q.opts.MaxLenApprox, map[string]interface{}{
		streamPayloadField: payload,
	}); err != nil {
		return fmt.Errorf("加入 Streams 队列失败: %w", err)
	}
	return nil
}

func encodeTaskMessage(message *TaskMessage) (string, error) {
	if message == nil {
		return "", fmt.Errorf("任务消息为空")
	}
	clone := *message
	clone.StreamID = ""
	clone.raw = ""
	data, err := json.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("序列化任务消息失败: %w", err)
	}
	return string(data), nil
}

func decodeStreamMessage(streamMessage cache.StreamMessage) (*TaskMessage, error) {
	payloadValue, ok := streamMessage.Values[streamPayloadField]
	if !ok {
		return nil, fmt.Errorf("Streams 消息缺少 payload")
	}

	var payload string
	switch value := payloadValue.(type) {
	case string:
		payload = value
	case []byte:
		payload = string(value)
	default:
		payload = fmt.Sprint(value)
	}

	var message TaskMessage
	if err := json.Unmarshal([]byte(payload), &message); err != nil {
		return nil, fmt.Errorf("反序列化 Streams 任务消息失败: %w", err)
	}
	message.StreamID = streamMessage.ID
	message.ProcessingAt = time.Now().Unix()
	message.raw = payload
	return &message, nil
}
