package queue

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/envutil"
	"caiyun/internal/models"
)

const (
	TaskStreamKey                      = "task:queue:stream"
	TaskStreamDelayedKey               = "task:queue:stream:delayed"
	TaskStreamDeadLetterKey            = "task:queue:stream:dead"
	DefaultStreamConsumerGroup         = "caiyun-workers"
	DefaultStreamMaxLenApprox          = 100000
	defaultStreamReadCount             = 1
	defaultStreamRecoverBatch          = 100
	streamPayloadField                 = "payload"
	streamDeadReasonField              = "reason"
	streamDeadFailedAtField            = "failed_at"
	streamDeadOriginalIDField          = "original_id"
	streamDeadOriginalDataField        = "original_payload"
	streamDeadOriginalLengthField      = "original_payload_length"
	streamDeadOriginalHashField        = "original_payload_sha256"
	streamDeadOriginalTruncatedField   = "original_payload_truncated"
	maxMalformedDeadLetterPayloadBytes = 64 * 1024
	// The live task stream is deletion-backed: successful ACKs atomically XDEL.
	// Never apply MAXLEN because Redis may trim undelivered or PEL entries.
	mainTaskStreamMaxLenApprox int64 = 0
	streamConsumerNameEnv            = "TASK_QUEUE_STREAM_CONSUMER"
	streamConsumerGroupEnv           = "TASK_QUEUE_STREAM_GROUP"
	streamKeyEnv                     = "TASK_QUEUE_STREAM_KEY"
	streamDelayedKeyEnv              = "TASK_QUEUE_STREAM_DELAYED_KEY"
	streamDeadKeyEnv                 = "TASK_QUEUE_STREAM_DEAD_KEY"
	streamMaxLenEnv                  = "TASK_QUEUE_STREAM_MAXLEN"
)

type streamQueueStore interface {
	XGroupCreateMkStream(stream, group, start string) error
	XAdd(stream string, maxLenApprox int64, values map[string]interface{}) (string, error)
	XAddBatch(stream string, maxLenApprox int64, values []map[string]interface{}) ([]string, error)
	XAddWithDedupe(stream string, maxLenApprox int64, item cache.StreamEnqueueItem, expiration time.Duration) (string, bool, error)
	XAddBatchWithDedupe(stream string, maxLenApprox int64, items []cache.StreamEnqueueItem, expiration time.Duration) ([]string, error)
	XReadGroup(group, consumer, stream, id string, count int64, block time.Duration) ([]cache.StreamMessage, error)
	XAckAndDelete(stream, group string, ids ...string) (int64, error)
	XRange(stream, start, end string, count int64) ([]cache.StreamMessage, error)
	XMoveEntryToStream(source, id, destination string, maxLenApprox int64, values map[string]interface{}) (string, bool, error)
	XDel(stream string, ids ...string) (int64, error)
	XMoveToStream(source, group, id, destination string, maxLenApprox int64, values map[string]interface{}) (string, bool, error)
	XMoveToZSet(source, group, id, destination, member string, score float64) (bool, error)
	XPromoteZSetToStream(source, destination, member string, maxScore float64, maxLenApprox int64, values map[string]interface{}) (string, bool, error)
	XAutoClaim(stream, group, consumer string, minIdle time.Duration, start string, count int64) ([]cache.StreamMessage, string, error)
	XRenewPending(stream, group, consumer, id string) (bool, error)
	XPendingCount(stream, group string) (int64, error)
	XLen(stream string) int64
	ZRangeByScore(key string, min, max string, count int64) ([]string, error)
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
		StreamKey:     envutil.String(streamKeyEnv, TaskStreamKey),
		DelayedKey:    envutil.String(streamDelayedKeyEnv, TaskStreamDelayedKey),
		DeadLetterKey: envutil.String(streamDeadKeyEnv, TaskStreamDeadLetterKey),
		ConsumerGroup: envutil.String(streamConsumerGroupEnv, DefaultStreamConsumerGroup),
		ConsumerName:  envutil.String(streamConsumerNameEnv, defaultStreamConsumerName()),
		MaxLenApprox:  envutil.Int64(streamMaxLenEnv, DefaultStreamMaxLenApprox),
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
			"delivery":             "consumer-group",
			"ack":                  "lua:xack+xdel",
			"live_stream_trimming": "disabled",
			"max_len_scope":        "dead-letter-only",
			// Multi-key Lua operations require all configured keys (including
			// dedupe keys) to share a hash tag when a Redis Cluster client is added.
			"redis_cluster": "requires-same-hash-slot",
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
	return q.EnqueueMessage(&message)
}

func (q *StreamTaskQueue) EnqueueMessage(message *TaskMessage) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if message.CreatedAt == 0 {
		message.CreatedAt = time.Now().Unix()
	}
	return q.enqueueMessage(message)
}

func (q *StreamTaskQueue) EnqueueBatch(accounts []*models.Account, taskType string) error {
	if len(accounts) == 0 {
		return nil
	}
	for index, account := range accounts {
		if account == nil {
			return fmt.Errorf("批量加入 Streams 队列失败: 第 %d 个账号为空", index+1)
		}
	}

	items := make([]cache.StreamEnqueueItem, 0, len(accounts))
	payloads := make([]map[string]interface{}, 0, len(accounts))
	var dedupeTTL time.Duration
	useDedupe := false
	for index, account := range accounts {
		message := TaskMessage{
			AccountID:  account.ID,
			UserID:     account.UserID,
			TaskType:   taskType,
			CreatedAt:  time.Now().Unix(),
			RetryCount: 0,
		}
		claim := prepareTaskEnqueueDedupe(&message)
		if index == 0 {
			useDedupe = claim.Enabled
			dedupeTTL = claim.TTL
		} else if claim.Enabled != useDedupe {
			return fmt.Errorf("批量加入 Streams 队列失败: 去重配置在构造批次时发生变化")
		}
		payload, err := encodeTaskMessage(&message)
		if err != nil {
			return fmt.Errorf("批量加入 Streams 队列失败: %w", err)
		}
		values := map[string]interface{}{streamPayloadField: payload}
		payloads = append(payloads, values)
		if claim.Enabled {
			items = append(items, cache.StreamEnqueueItem{
				Values:      values,
				DedupeKey:   claim.Key,
				DedupeValue: claim.Value,
			})
		}
	}

	if err := q.ensureGroup(); err != nil {
		return err
	}
	if useDedupe {
		if _, err := q.cache.XAddBatchWithDedupe(q.opts.StreamKey, mainTaskStreamMaxLenApprox, items, dedupeTTL); err != nil {
			return fmt.Errorf("批量加入 Streams 队列失败: %w", err)
		}
		return nil
	}
	if _, err := q.cache.XAddBatch(q.opts.StreamKey, mainTaskStreamMaxLenApprox, payloads); err != nil {
		return fmt.Errorf("批量加入 Streams 队列失败: %w", err)
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
		// Redis XREADGROUP returns redis.Nil for a normal BLOCK timeout. The
		// cache layer converts it to a text error because it cannot import this
		// package's sentinel without an import cycle. Normalize it here so the
		// Worker can distinguish an idle queue from an actual Redis failure.
		if strings.Contains(err.Error(), ErrQueueTimeout.Error()) {
			return nil, ErrQueueTimeout
		}
		return nil, err
	}
	if len(messages) == 0 {
		return nil, ErrQueueTimeout
	}
	message, decodeErr := decodeStreamMessage(messages[0])
	if decodeErr == nil {
		return message, nil
	}
	if err := q.moveMalformedToDeadLetter(messages[0], decodeErr); err != nil {
		return nil, fmt.Errorf("%v；异常消息转入死信队列失败: %w", decodeErr, err)
	}
	return nil, fmt.Errorf("%v；异常消息已转入死信队列", decodeErr)
}

func (q *StreamTaskQueue) Ack(message *TaskMessage) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if message.StreamID == "" {
		return fmt.Errorf("Streams 消息缺少 StreamID")
	}
	acked, err := q.cache.XAckAndDelete(q.opts.StreamKey, q.opts.ConsumerGroup, message.StreamID)
	if err != nil {
		return fmt.Errorf("原子确认并删除 Streams 任务失败: %w", err)
	}
	if acked != 1 {
		return fmt.Errorf("原子确认并删除 Streams 任务冲突: 消息不在当前 consumer group PEL 中 (acked=%d)", acked)
	}
	return nil
}

func (q *StreamTaskQueue) Requeue(message *TaskMessage) error {
	return q.RequeueDelayed(message, retryBackoff(message))
}

// RenewVisibility resets the PEL idle time only when this queue consumer still
// owns the exact pending entry.  It deliberately does not XCLAIM a delivery
// from another consumer: doing so could steal a task after a stale recovery
// has already handed it to a new Worker.
func (q *StreamTaskQueue) RenewVisibility(message *TaskMessage) (bool, error) {
	if message == nil {
		return false, fmt.Errorf("任务消息为空")
	}
	if strings.TrimSpace(message.StreamID) == "" {
		return false, fmt.Errorf("Streams 消息缺少 StreamID")
	}
	if err := q.ensureGroup(); err != nil {
		return false, err
	}
	renewed, err := q.cache.XRenewPending(q.opts.StreamKey, q.opts.ConsumerGroup, q.opts.ConsumerName, message.StreamID)
	if err != nil {
		return false, fmt.Errorf("续约 Streams 处理中任务失败: %w", err)
	}
	return renewed, nil
}

func (q *StreamTaskQueue) RequeueDelayed(message *TaskMessage, delay time.Duration) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if message.StreamID == "" {
		return fmt.Errorf("Streams 消息缺少 StreamID")
	}
	if err := q.ensureGroup(); err != nil {
		return err
	}
	if delay < 0 {
		delay = 0
	}

	payload, err := encodeTaskMessage(message)
	if err != nil {
		return err
	}
	if delay == 0 {
		_, moved, err := q.cache.XMoveToStream(
			q.opts.StreamKey,
			q.opts.ConsumerGroup,
			message.StreamID,
			q.opts.StreamKey,
			mainTaskStreamMaxLenApprox,
			map[string]interface{}{streamPayloadField: payload},
		)
		if err != nil {
			return fmt.Errorf("原子重入 Streams 队列失败: %w", err)
		}
		if !moved {
			return fmt.Errorf("原子重入 Streams 队列失败: 原始处理中消息不存在")
		}
		return nil
	}

	availableAt := time.Now().Add(delay).Unix()
	moved, err := q.cache.XMoveToZSet(
		q.opts.StreamKey,
		q.opts.ConsumerGroup,
		message.StreamID,
		q.opts.DelayedKey,
		payload,
		float64(availableAt),
	)
	if err != nil {
		return fmt.Errorf("原子加入 Streams 延迟队列失败: %w", err)
	}
	if !moved {
		return fmt.Errorf("原子加入 Streams 延迟队列失败: 原始处理中消息不存在")
	}
	return nil
}

func (q *StreamTaskQueue) DeadLetter(message *TaskMessage, reason string) error {
	if message == nil {
		return fmt.Errorf("任务消息为空")
	}
	if message.StreamID == "" {
		return fmt.Errorf("Streams 消息缺少 StreamID")
	}
	if err := q.ensureGroup(); err != nil {
		return err
	}

	payload, err := encodeTaskMessage(message)
	if err != nil {
		return err
	}
	_, moved, err := q.cache.XMoveToStream(
		q.opts.StreamKey,
		q.opts.ConsumerGroup,
		message.StreamID,
		q.opts.DeadLetterKey,
		q.opts.MaxLenApprox,
		map[string]interface{}{
			streamDeadOriginalIDField:   message.StreamID,
			streamDeadReasonField:       reason,
			streamDeadFailedAtField:     time.Now().Unix(),
			streamDeadOriginalDataField: payload,
		},
	)
	if err != nil {
		return fmt.Errorf("原子写入 Streams 死信队列失败: %w", err)
	}
	if !moved {
		return fmt.Errorf("原子写入 Streams 死信队列失败: 原始处理中消息不存在")
	}
	return nil
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
			if deadErr := q.moveMalformedToDeadLetter(streamMessage, err); deadErr != nil {
				return recovered, fmt.Errorf("恢复异常 Streams 消息时写入死信队列失败: %w", deadErr)
			}
			continue
		}
		payload, err := encodeTaskMessage(message)
		if err != nil {
			return recovered, err
		}
		_, moved, err := q.cache.XMoveToStream(
			q.opts.StreamKey,
			q.opts.ConsumerGroup,
			message.StreamID,
			q.opts.StreamKey,
			mainTaskStreamMaxLenApprox,
			map[string]interface{}{streamPayloadField: payload},
		)
		if err != nil {
			return recovered, err
		}
		if moved {
			recovered++
		}
	}

	return recovered, nil
}

func (q *StreamTaskQueue) PromoteDueDelayed(limit int64) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	if err := q.ensureGroup(); err != nil {
		return 0, err
	}

	now := time.Now().Unix()
	items, err := q.cache.ZRangeByScore(q.opts.DelayedKey, "-inf", fmt.Sprintf("%d", now), limit)
	if err != nil {
		return 0, err
	}

	promoted := 0
	for _, item := range items {
		_, moved, err := q.cache.XPromoteZSetToStream(
			q.opts.DelayedKey,
			q.opts.StreamKey,
			item,
			float64(now),
			mainTaskStreamMaxLenApprox,
			map[string]interface{}{streamPayloadField: item},
		)
		if err != nil {
			return promoted, err
		}
		if moved {
			promoted++
		}
	}

	return promoted, nil
}

func (q *StreamTaskQueue) moveMalformedToDeadLetter(streamMessage cache.StreamMessage, cause error) error {
	if strings.TrimSpace(streamMessage.ID) == "" {
		return fmt.Errorf("异常 Streams 消息缺少 ID")
	}
	reason := "无法解析 Streams 任务消息"
	if cause != nil {
		reason = cause.Error()
	}
	if len(reason) > 512 {
		reason = reason[:512]
	}
	originalPayload := ""
	if value, ok := streamMessage.Values[streamPayloadField]; ok {
		switch typed := value.(type) {
		case string:
			originalPayload = typed
		case []byte:
			originalPayload = string(typed)
		default:
			originalPayload = fmt.Sprint(typed)
		}
	} else if encoded, err := json.Marshal(streamMessage.Values); err == nil {
		originalPayload = string(encoded)
	}
	originalPayload, originalLength, originalHash, originalTruncated := boundMalformedDeadLetterPayload(originalPayload)
	_, moved, err := q.cache.XMoveToStream(
		q.opts.StreamKey,
		q.opts.ConsumerGroup,
		streamMessage.ID,
		q.opts.DeadLetterKey,
		q.opts.MaxLenApprox,
		map[string]interface{}{
			streamDeadOriginalIDField:        streamMessage.ID,
			streamDeadReasonField:            reason,
			streamDeadFailedAtField:          time.Now().Unix(),
			streamDeadOriginalDataField:      originalPayload,
			streamDeadOriginalLengthField:    originalLength,
			streamDeadOriginalHashField:      originalHash,
			streamDeadOriginalTruncatedField: originalTruncated,
		},
	)
	if err != nil {
		return err
	}
	if !moved {
		return fmt.Errorf("原始处理中消息不存在")
	}
	return nil
}

func boundMalformedDeadLetterPayload(payload string) (captured string, originalLength int, sha256Hex string, truncated bool) {
	data := []byte(payload)
	originalLength = len(data)
	sum := sha256.Sum256(data)
	sha256Hex = fmt.Sprintf("%x", sum[:])
	if len(data) > maxMalformedDeadLetterPayloadBytes {
		data = data[:maxMalformedDeadLetterPayloadBytes]
		truncated = true
	}
	return string(data), originalLength, sha256Hex, truncated
}

func streamDeadLetterMessage(message cache.StreamMessage) (*DeadLetterMessage, *TaskMessage, error) {
	reason := strings.TrimSpace(fmt.Sprint(message.Values[streamDeadReasonField]))
	failedAt := int64(0)
	if raw, ok := message.Values[streamDeadFailedAtField]; ok {
		if parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(raw)), 10, 64); err == nil {
			failedAt = parsed
		}
	}
	payload, ok := message.Values[streamDeadOriginalDataField]
	if !ok {
		return &DeadLetterMessage{ID: message.ID, Reason: reason, FailedAt: time.Unix(failedAt, 0).UTC(), Malformed: true}, nil, nil
	}
	var task TaskMessage
	if err := json.Unmarshal([]byte(fmt.Sprint(payload)), &task); err != nil {
		return &DeadLetterMessage{ID: message.ID, Reason: reason, FailedAt: time.Unix(failedAt, 0).UTC(), Malformed: true}, nil, nil
	}
	return &DeadLetterMessage{ID: message.ID, Reason: reason, FailedAt: time.Unix(failedAt, 0).UTC(), Task: &task}, &task, nil
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

func (q *StreamTaskQueue) ListDeadLetters(limit int) ([]*DeadLetterMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	messages, err := q.cache.XRange(q.opts.DeadLetterKey, "-", "+", int64(limit))
	if err != nil {
		return nil, err
	}
	items := make([]*DeadLetterMessage, 0, len(messages))
	for _, message := range messages {
		item, _, parseErr := streamDeadLetterMessage(message)
		if parseErr == nil {
			items = append(items, item)
		}
	}
	return items, nil
}

func (q *StreamTaskQueue) GetDeadLetter(id string) (*DeadLetterMessage, error) {
	messages, err := q.cache.XRange(q.opts.DeadLetterKey, id, id, 1)
	if err != nil {
		return nil, err
	}
	if len(messages) != 1 {
		return nil, ErrDeadLetterNotFound
	}
	item, _, err := streamDeadLetterMessage(messages[0])
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (q *StreamTaskQueue) ReplayDeadLetter(id string) (*TaskMessage, error) {
	messages, err := q.cache.XRange(q.opts.DeadLetterKey, id, id, 1)
	if err != nil {
		return nil, err
	}
	if len(messages) != 1 {
		return nil, ErrDeadLetterNotFound
	}
	item, task, err := streamDeadLetterMessage(messages[0])
	if err != nil {
		return nil, err
	}
	if item.Malformed || task == nil {
		return nil, fmt.Errorf("malformed dead-letter entry cannot be replayed")
	}
	if task.OperationID != "" {
		return nil, ErrOperationDeadLetterReplay
	}
	message := resetReplayMessage(task)
	payload, err := encodeTaskMessage(message)
	if err != nil {
		return nil, err
	}
	_, moved, err := q.cache.XMoveEntryToStream(
		q.opts.DeadLetterKey, id, q.opts.StreamKey, mainTaskStreamMaxLenApprox,
		map[string]interface{}{streamPayloadField: payload},
	)
	if err != nil {
		return nil, err
	}
	if !moved {
		return nil, ErrDeadLetterNotFound
	}
	return message, nil
}

func (q *StreamTaskQueue) ArchiveDeadLetter(id string) error {
	deleted, err := q.cache.XDel(q.opts.DeadLetterKey, id)
	if err != nil {
		return err
	}
	if deleted != 1 {
		return ErrDeadLetterNotFound
	}
	return nil
}

func (q *StreamTaskQueue) Clear() error {
	if err := q.cache.Del(q.opts.StreamKey, q.opts.DelayedKey, q.opts.DeadLetterKey); err != nil {
		return err
	}
	if err := clearTaskDedupeKeys(q.cache); err != nil {
		return err
	}
	return q.ensureGroup()
}

func (q *StreamTaskQueue) enqueueMessage(message *TaskMessage) error {
	claim := prepareTaskEnqueueDedupe(message)
	payload, err := encodeTaskMessage(message)
	if err != nil {
		return err
	}
	if err := q.ensureGroup(); err != nil {
		return err
	}
	values := map[string]interface{}{streamPayloadField: payload}
	if claim.Enabled {
		_, _, err := q.cache.XAddWithDedupe(q.opts.StreamKey, mainTaskStreamMaxLenApprox, cache.StreamEnqueueItem{
			Values:      values,
			DedupeKey:   claim.Key,
			DedupeValue: claim.Value,
		}, claim.TTL)
		if err != nil {
			return fmt.Errorf("原子加入 Streams 队列失败: %w", err)
		}
		return nil
	}
	if _, err := q.cache.XAdd(q.opts.StreamKey, mainTaskStreamMaxLenApprox, values); err != nil {
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
