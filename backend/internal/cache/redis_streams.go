// Redis Streams is intentionally isolated from the generic cache/list API.
// Keeping stream consumer-group, atomic enqueue, delayed delivery and claim
// primitives together makes queue invariants independently reviewable.
package cache

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

type StreamMessage struct {
	ID     string
	Values map[string]interface{}
}

// StreamEnqueueItem describes one Streams message and the Redis key used to
// deduplicate its initial enqueue. The dedupe key is written only after XADD
// succeeds, in the same Lua script, so a failed enqueue cannot leave a claim
// that suppresses the task.
type StreamEnqueueItem struct {
	Values      map[string]interface{}
	DedupeKey   string
	DedupeValue interface{}
}

func (r *RedisCache) XGroupCreateMkStream(stream, group, start string) error {
	if start == "" {
		start = "0"
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	err := r.client.XGroupCreateMkStream(ctx, stream, group, start).Err()
	if err != nil && strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return err
}

func (r *RedisCache) XAdd(stream string, maxLenApprox int64, values map[string]interface{}) (string, error) {
	args := &redis.XAddArgs{
		Stream: stream,
		Values: values,
	}
	if maxLenApprox > 0 {
		args.MaxLenApprox = maxLenApprox
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.XAdd(ctx, args).Result()
}

func (r *RedisCache) XAddBatch(stream string, maxLenApprox int64, values []map[string]interface{}) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	ctx, cancel := r.operationContext()
	defer cancel()

	pipe := r.client.TxPipeline()
	commands := make([]*redis.StringCmd, 0, len(values))
	for _, item := range values {
		args := &redis.XAddArgs{
			Stream: stream,
			Values: item,
		}
		if maxLenApprox > 0 {
			args.MaxLenApprox = maxLenApprox
		}
		commands = append(commands, pipe.XAdd(ctx, args))
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(commands))
	for _, command := range commands {
		id, err := command.Result()
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

const xAddWithDedupeScript = `
local ttl = tonumber(ARGV[1])
local maxlen = tonumber(ARGV[2])
local argpos = 3
local results = {}

local function add_to_stream(stream, fields)
	if maxlen > 0 then
		return redis.call('XADD', stream, 'MAXLEN', '~', maxlen, '*', unpack(fields))
	end
	return redis.call('XADD', stream, '*', unpack(fields))
end

for keypos = 2, #KEYS do
	local dedupe_value = ARGV[argpos]
	local field_count = tonumber(ARGV[argpos + 1])
	argpos = argpos + 2
	local fields = {}
	for field_index = 1, field_count do
		table.insert(fields, ARGV[argpos])
		table.insert(fields, ARGV[argpos + 1])
		argpos = argpos + 2
	end

	if redis.call('EXISTS', KEYS[keypos]) == 1 then
		table.insert(results, '')
	else
		-- XADD intentionally happens before the claim. Redis scripts do not roll
		-- back earlier writes after a later command error; PSETEX is deterministic
		-- here, so this ordering cannot suppress a task that was never enqueued.
		local id = add_to_stream(KEYS[1], fields)
		redis.call('PSETEX', KEYS[keypos], ttl, dedupe_value)
		table.insert(results, id)
	end
end
return results
`

// XAddWithDedupe atomically checks a dedupe key, appends a Streams message and
// records the claim. added=false means another enqueue already owns the claim.
func (r *RedisCache) XAddWithDedupe(stream string, maxLenApprox int64, item StreamEnqueueItem, expiration time.Duration) (string, bool, error) {
	ids, err := r.XAddBatchWithDedupe(stream, maxLenApprox, []StreamEnqueueItem{item}, expiration)
	if err != nil {
		return "", false, err
	}
	if len(ids) != 1 {
		return "", false, fmt.Errorf("atomic Streams enqueue returned %d ids, want 1", len(ids))
	}
	return ids[0], ids[0] != "", nil
}

// XAddBatchWithDedupe performs the initial dedupe claim and XADD for every item
// in one Lua execution. The returned slice aligns with items; an empty ID marks
// an item skipped because its dedupe key already existed.
func (r *RedisCache) XAddBatchWithDedupe(stream string, maxLenApprox int64, items []StreamEnqueueItem, expiration time.Duration) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	if strings.TrimSpace(stream) == "" {
		return nil, fmt.Errorf("Streams key is empty")
	}
	if expiration <= 0 {
		return nil, fmt.Errorf("Streams dedupe expiration must be positive")
	}
	ttlMillis := expiration.Milliseconds()
	if ttlMillis <= 0 {
		return nil, fmt.Errorf("Streams dedupe expiration must be at least 1ms")
	}

	keys := make([]string, 1, len(items)+1)
	keys[0] = stream
	args := make([]interface{}, 0, 2+len(items)*5)
	args = append(args, ttlMillis, maxLenApprox)
	for index, item := range items {
		if strings.TrimSpace(item.DedupeKey) == "" {
			return nil, fmt.Errorf("Streams dedupe key at index %d is empty", index)
		}
		fieldArgs, err := orderedStreamValueArgs(item.Values)
		if err != nil {
			return nil, fmt.Errorf("Streams values at index %d: %w", index, err)
		}
		keys = append(keys, item.DedupeKey)
		args = append(args, fmt.Sprint(item.DedupeValue), len(item.Values))
		args = append(args, fieldArgs...)
	}

	ctx, cancel := r.operationContext()
	defer cancel()
	raw, err := r.client.Eval(ctx, xAddWithDedupeScript, keys, args...).Result()
	if err != nil {
		return nil, err
	}
	parts, ok := asInterfaceSlice(raw)
	if !ok || len(parts) != len(items) {
		return nil, fmt.Errorf("unexpected atomic Streams enqueue reply %T", raw)
	}
	ids := make([]string, len(parts))
	for index, part := range parts {
		ids[index] = valueToString(part)
	}
	return ids, nil
}

const xAckAndDeleteScript = `
local total = 0
for index = 2, #ARGV do
	local acked = redis.call('XACK', KEYS[1], ARGV[1], ARGV[index])
	if acked > 0 then
		redis.call('XDEL', KEYS[1], ARGV[index])
		total = total + acked
	end
end
return total
`

// XAckAndDelete atomically acknowledges and removes pending Streams entries.
func (r *RedisCache) XAckAndDelete(stream, group string, ids ...string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	args := make([]interface{}, 0, len(ids)+1)
	args = append(args, group)
	for _, id := range ids {
		args = append(args, id)
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	value, err := r.client.Eval(ctx, xAckAndDeleteScript, []string{stream}, args...).Int()
	return int64(value), err
}

const xMoveToStreamScript = `
local pending = redis.call('XPENDING', KEYS[1], ARGV[1], ARGV[2], ARGV[2], 1)
if #pending == 0 then
	return {0, ''}
end

local maxlen = tonumber(ARGV[3])
local field_count = tonumber(ARGV[4])
local fields = {}
local argpos = 5
for field_index = 1, field_count do
	table.insert(fields, ARGV[argpos])
	table.insert(fields, ARGV[argpos + 1])
	argpos = argpos + 2
end

local new_id
if maxlen > 0 then
	new_id = redis.call('XADD', KEYS[2], 'MAXLEN', '~', maxlen, '*', unpack(fields))
else
	new_id = redis.call('XADD', KEYS[2], '*', unpack(fields))
end
redis.call('XACK', KEYS[1], ARGV[1], ARGV[2])
redis.call('XDEL', KEYS[1], ARGV[2])
return {1, new_id}
`

// XMoveToStream atomically writes a replacement/dead-letter Streams entry and
// then acknowledges and deletes its source pending entry.
func (r *RedisCache) XMoveToStream(source, group, id, destination string, maxLenApprox int64, values map[string]interface{}) (string, bool, error) {
	fieldArgs, err := orderedStreamValueArgs(values)
	if err != nil {
		return "", false, err
	}
	args := make([]interface{}, 0, 4+len(fieldArgs))
	args = append(args, group, id, maxLenApprox, len(values))
	args = append(args, fieldArgs...)
	ctx, cancel := r.operationContext()
	defer cancel()
	raw, err := r.client.Eval(ctx, xMoveToStreamScript, []string{source, destination}, args...).Result()
	if err != nil {
		return "", false, err
	}
	return parseAtomicStreamResult(raw)
}

const xMoveToZSetScript = `
local pending = redis.call('XPENDING', KEYS[1], ARGV[1], ARGV[2], ARGV[2], 1)
if #pending == 0 then
	return 0
end
redis.call('ZADD', KEYS[2], ARGV[3], ARGV[4])
redis.call('XACK', KEYS[1], ARGV[1], ARGV[2])
redis.call('XDEL', KEYS[1], ARGV[2])
return 1
`

// XMoveToZSet atomically schedules a pending Streams entry in a sorted set and
// then acknowledges/deletes the source entry.
func (r *RedisCache) XMoveToZSet(source, group, id, destination, member string, score float64) (bool, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	result, err := r.client.Eval(
		ctx,
		xMoveToZSetScript,
		[]string{source, destination},
		group,
		id,
		strconv.FormatFloat(score, 'f', -1, 64),
		member,
	).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

const xPromoteZSetToStreamScript = `
local score = redis.call('ZSCORE', KEYS[1], ARGV[1])
if not score or tonumber(score) > tonumber(ARGV[2]) then
	return {0, ''}
end

local maxlen = tonumber(ARGV[3])
local field_count = tonumber(ARGV[4])
local fields = {}
local argpos = 5
for field_index = 1, field_count do
	table.insert(fields, ARGV[argpos])
	table.insert(fields, ARGV[argpos + 1])
	argpos = argpos + 2
end

local new_id
if maxlen > 0 then
	new_id = redis.call('XADD', KEYS[2], 'MAXLEN', '~', maxlen, '*', unpack(fields))
else
	new_id = redis.call('XADD', KEYS[2], '*', unpack(fields))
end
redis.call('ZREM', KEYS[1], ARGV[1])
return {1, new_id}
`

// XPromoteZSetToStream atomically promotes a due delayed member into a Stream.
// The due score is checked again inside the script to avoid racing reschedules.
func (r *RedisCache) XPromoteZSetToStream(source, destination, member string, maxScore float64, maxLenApprox int64, values map[string]interface{}) (string, bool, error) {
	fieldArgs, err := orderedStreamValueArgs(values)
	if err != nil {
		return "", false, err
	}
	args := make([]interface{}, 0, 4+len(fieldArgs))
	args = append(
		args,
		member,
		strconv.FormatFloat(maxScore, 'f', -1, 64),
		maxLenApprox,
		len(values),
	)
	args = append(args, fieldArgs...)
	ctx, cancel := r.operationContext()
	defer cancel()
	raw, err := r.client.Eval(ctx, xPromoteZSetToStreamScript, []string{source, destination}, args...).Result()
	if err != nil {
		return "", false, err
	}
	return parseAtomicStreamResult(raw)
}

func orderedStreamValueArgs(values map[string]interface{}) ([]interface{}, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("Streams values are empty")
	}
	fieldNames := make([]string, 0, len(values))
	for field, value := range values {
		if strings.TrimSpace(field) == "" {
			return nil, fmt.Errorf("Streams field name is empty")
		}
		if value == nil {
			return nil, fmt.Errorf("Streams field %q has nil value", field)
		}
		fieldNames = append(fieldNames, field)
	}
	sort.Strings(fieldNames)
	args := make([]interface{}, 0, len(fieldNames)*2)
	for _, field := range fieldNames {
		args = append(args, field, values[field])
	}
	return args, nil
}

func parseAtomicStreamResult(raw interface{}) (string, bool, error) {
	parts, ok := asInterfaceSlice(raw)
	if !ok || len(parts) != 2 {
		return "", false, fmt.Errorf("unexpected atomic Streams move reply %T", raw)
	}
	moved := valueToString(parts[0]) == "1"
	id := valueToString(parts[1])
	if moved && id == "" {
		return "", false, fmt.Errorf("atomic Streams move returned no destination id")
	}
	return id, moved, nil
}
func (r *RedisCache) XReadGroup(group, consumer, stream, id string, count int64, block time.Duration) ([]StreamMessage, error) {
	if id == "" {
		id = ">"
	}
	if count <= 0 {
		count = 1
	}
	ctx, cancel := r.operationContext(block)
	defer cancel()
	result, err := r.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, id},
		Count:    count,
		Block:    block,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("队列超时")
		}
		return nil, err
	}
	return flattenStreamMessages(result), nil
}

func (r *RedisCache) XAck(stream, group string, ids ...string) (int64, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.XAck(ctx, stream, group, ids...).Result()
}

func (r *RedisCache) XDel(stream string, ids ...string) (int64, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.XDel(ctx, stream, ids...).Result()
}

// XRange reads durable dead-letter entries without joining a consumer group.
func (r *RedisCache) XRange(stream, start, end string, count int64) ([]StreamMessage, error) {
	if start == "" {
		start = "-"
	}
	if end == "" {
		end = "+"
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	messages, err := r.client.XRangeN(ctx, stream, start, end, count).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return flattenSingleStreamMessages(messages), nil
}

const xMoveEntryToStreamScript = `
local source_entry = redis.call('XRANGE', KEYS[1], ARGV[1], ARGV[1], 'COUNT', 1)
if #source_entry == 0 then
	return {0, ''}
end
local maxlen = tonumber(ARGV[2])
local field_count = tonumber(ARGV[3])
local fields = {}
local argpos = 4
for field_index = 1, field_count do
	table.insert(fields, ARGV[argpos])
	table.insert(fields, ARGV[argpos + 1])
	argpos = argpos + 2
end
local new_id
if maxlen > 0 then
	new_id = redis.call('XADD', KEYS[2], 'MAXLEN', '~', maxlen, '*', unpack(fields))
else
	new_id = redis.call('XADD', KEYS[2], '*', unpack(fields))
end
redis.call('XDEL', KEYS[1], ARGV[1])
return {1, new_id}
`

// XMoveEntryToStream atomically republishes a dead-letter entry and deletes
// the source entry only after the destination XADD has succeeded.
func (r *RedisCache) XMoveEntryToStream(source, id, destination string, maxLenApprox int64, values map[string]interface{}) (string, bool, error) {
	fieldArgs, err := orderedStreamValueArgs(values)
	if err != nil {
		return "", false, err
	}
	args := make([]interface{}, 0, 3+len(fieldArgs))
	args = append(args, id, maxLenApprox, len(values))
	args = append(args, fieldArgs...)
	ctx, cancel := r.operationContext()
	defer cancel()
	raw, err := r.client.Eval(ctx, xMoveEntryToStreamScript, []string{source, destination}, args...).Result()
	if err != nil {
		return "", false, err
	}
	return parseAtomicStreamResult(raw)
}

func (r *RedisCache) XAutoClaim(stream, group, consumer string, minIdle time.Duration, start string, count int64) ([]StreamMessage, string, error) {
	if start == "" {
		start = "0-0"
	}
	if count <= 0 {
		count = 100
	}
	// github.com/go-redis/redis/v8 的 XAutoClaim 结果解析在部分 Redis 7.x
	// 环境下仍按 Redis 6.2 的两段响应解析，而 Redis 7 会返回第三段
	// deleted IDs，导致报错：got 3, wanted 2。这里使用原始 DO 命令并兼容
	// 两段/三段响应，保证 CI 与生产 Redis 版本差异下都可恢复 pending 消息。
	ctx, cancel := r.operationContext()
	defer cancel()
	raw, err := r.client.Do(
		ctx,
		"XAUTOCLAIM",
		stream,
		group,
		consumer,
		int64(minIdle/time.Millisecond),
		start,
		"COUNT",
		count,
	).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, start, nil
		}
		return nil, start, err
	}
	return parseXAutoClaimReply(raw)
}

const xRenewPendingScript = `
local pending = redis.call('XPENDING', KEYS[1], ARGV[1], ARGV[2], ARGV[2], 1, ARGV[3])
if #pending == 0 or pending[1][1] ~= ARGV[2] then
	return 0
end
local claimed = redis.call('XCLAIM', KEYS[1], ARGV[1], ARGV[3], 0, ARGV[2], 'IDLE', 0, 'JUSTID')
if #claimed == 0 then
	return 0
end
return 1
`

// XRenewPending resets the idle age of an entry in a consumer group's PEL if
// and only if it still belongs to consumer.  Checking ownership and resetting
// IDLE happen in one Lua invocation so an active Worker never steals a
// delivery that stale recovery has transferred to a different consumer.
func (r *RedisCache) XRenewPending(stream, group, consumer, id string) (bool, error) {
	if strings.TrimSpace(stream) == "" || strings.TrimSpace(group) == "" || strings.TrimSpace(consumer) == "" || strings.TrimSpace(id) == "" {
		return false, fmt.Errorf("Streams pending renewal requires stream, group, consumer and id")
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	raw, err := r.client.Eval(ctx, xRenewPendingScript, []string{stream}, group, id, consumer).Result()
	if err != nil {
		if err == redis.Nil || strings.Contains(err.Error(), "NOGROUP") {
			return false, nil
		}
		return false, err
	}
	return valueToString(raw) == "1", nil
}

func (r *RedisCache) XPendingCount(stream, group string) (int64, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	pending, err := r.client.XPending(ctx, stream, group).Result()
	if err != nil {
		if err == redis.Nil || strings.Contains(err.Error(), "NOGROUP") {
			return 0, nil
		}
		return 0, err
	}
	return pending.Count, nil
}

func (r *RedisCache) XLen(stream string) int64 {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.XLen(ctx, stream).Val()
}

func flattenStreamMessages(streams []redis.XStream) []StreamMessage {
	messages := make([]StreamMessage, 0)
	for _, stream := range streams {
		messages = append(messages, flattenSingleStreamMessages(stream.Messages)...)
	}
	return messages
}

func flattenSingleStreamMessages(messages []redis.XMessage) []StreamMessage {
	result := make([]StreamMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, StreamMessage{
			ID:     message.ID,
			Values: message.Values,
		})
	}
	return result
}

func parseXAutoClaimReply(raw interface{}) ([]StreamMessage, string, error) {
	parts, ok := asInterfaceSlice(raw)
	if !ok || len(parts) < 2 {
		return nil, "", fmt.Errorf("解析 XAUTOCLAIM 响应失败: unexpected reply %T", raw)
	}

	nextStart := valueToString(parts[0])
	messageParts, ok := asInterfaceSlice(parts[1])
	if !ok {
		return nil, nextStart, fmt.Errorf("解析 XAUTOCLAIM 消息列表失败: unexpected type %T", parts[1])
	}

	messages := make([]StreamMessage, 0, len(messageParts))
	for _, item := range messageParts {
		message, ok := parseRawStreamMessage(item)
		if !ok {
			continue
		}
		messages = append(messages, message)
	}
	return messages, nextStart, nil
}

func parseRawStreamMessage(raw interface{}) (StreamMessage, bool) {
	parts, ok := asInterfaceSlice(raw)
	if !ok || len(parts) < 2 {
		return StreamMessage{}, false
	}

	id := valueToString(parts[0])
	if id == "" {
		return StreamMessage{}, false
	}

	fieldParts, ok := asInterfaceSlice(parts[1])
	if !ok {
		return StreamMessage{}, false
	}

	values := make(map[string]interface{}, len(fieldParts)/2)
	for i := 0; i+1 < len(fieldParts); i += 2 {
		key := valueToString(fieldParts[i])
		if key == "" {
			continue
		}
		values[key] = normalizeRedisScalar(fieldParts[i+1])
	}
	return StreamMessage{ID: id, Values: values}, true
}

func asInterfaceSlice(value interface{}) ([]interface{}, bool) {
	switch v := value.(type) {
	case []interface{}:
		return v, true
	case []string:
		out := make([]interface{}, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	case [][]interface{}:
		out := make([]interface{}, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out, true
	default:
		return nil, false
	}
}

func normalizeRedisScalar(value interface{}) interface{} {
	switch v := value.(type) {
	case []byte:
		return string(v)
	default:
		return v
	}
}

func valueToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

// ScanKeysByPrefix 按前缀扫描 Redis 键，避免使用阻塞式 KEYS 命令。
