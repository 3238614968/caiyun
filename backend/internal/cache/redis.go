package cache

import (
	"caiyun/internal/envutil"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

// RedisConfig Redis配置结构体
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// NewRedisCache 创建Redis缓存实例
func NewRedisCache(config RedisConfig) (*RedisCache, error) {
	addr := fmt.Sprintf("%s:%s", config.Host, config.Port)
	return NewRedisClient(addr, config.Password, config.DB)
}

type RedisCache struct {
	client           *redis.Client
	ctx              context.Context
	cancel           context.CancelFunc
	operationTimeout time.Duration
}

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

func NewRedisClient(addr, password string, db int) (*RedisCache, error) {
	baseCtx, cancel := context.WithCancel(context.Background())
	operationTimeout := redisDurationFromEnv("REDIS_OPERATION_TIMEOUT", 5*time.Second)
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     redisIntFromEnv("REDIS_POOL_SIZE", 50),
		MinIdleConns: redisIntFromEnv("REDIS_MIN_IDLE_CONNS", 10),
		DialTimeout:  redisDurationFromEnv("REDIS_DIAL_TIMEOUT", 5*time.Second),
		ReadTimeout:  redisDurationFromEnv("REDIS_READ_TIMEOUT", 3*time.Second),
		WriteTimeout: redisDurationFromEnv("REDIS_WRITE_TIMEOUT", 3*time.Second),
		PoolTimeout:  redisDurationFromEnv("REDIS_POOL_TIMEOUT", 4*time.Second),
		IdleTimeout:  redisDurationFromEnv("REDIS_IDLE_TIMEOUT", 5*time.Minute),
	})

	ctx, pingCancel := context.WithTimeout(baseCtx, operationTimeout)
	defer pingCancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		cancel()
		return nil, fmt.Errorf("Redis连接失败: %w", err)
	}

	return &RedisCache{
		client:           rdb,
		ctx:              baseCtx,
		cancel:           cancel,
		operationTimeout: operationTimeout,
	}, nil
}

// Ping checks Redis while respecting the caller deadline. Readiness probes use
// this instead of an operation with an internal background context so their
// HTTP timeout remains a hard upper bound.
func (r *RedisCache) Ping(parent context.Context) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("Redis client is nil")
	}
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return err
		}
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx := parent
	cancel := func() {}
	if _, hasDeadline := parent.Deadline(); !hasDeadline {
		timeout := r.operationTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		ctx, cancel = context.WithTimeout(parent, timeout)
	}
	defer cancel()
	return r.client.Ping(ctx).Err()
}

func (r *RedisCache) operationContext(extra ...time.Duration) (context.Context, context.CancelFunc) {
	timeout := r.operationTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	for _, duration := range extra {
		if duration > 0 {
			timeout += duration
		}
	}
	base := r.ctx
	if base == nil {
		base = context.Background()
	}
	return context.WithTimeout(base, timeout)
}

func redisIntFromEnv(key string, fallback int) int {
	value := envutil.Int(key, fallback)
	if value <= 0 {
		return fallback
	}
	return value
}

func redisDurationFromEnv(key string, fallback time.Duration) time.Duration {
	return envutil.Duration(key, fallback)
}

func (r *RedisCache) Set(key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.Set(ctx, key, data, expiration).Err()
}

func (r *RedisCache) SetNX(key string, value interface{}, expiration time.Duration) (bool, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.SetNX(ctx, key, value, expiration).Result()
}

func (r *RedisCache) DelIfValue(key, value string) (bool, error) {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	ctx, cancel := r.operationContext()
	defer cancel()
	deleted, err := r.client.Eval(ctx, script, []string{key}, value).Int()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (r *RedisCache) Get(key string, dest interface{}) error {
	ctx, cancel := r.operationContext()
	defer cancel()
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (r *RedisCache) Del(keys ...string) error {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.Del(ctx, keys...).Err()
}

func (r *RedisCache) Exists(keys ...string) (int64, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.Exists(ctx, keys...).Result()
}

func (r *RedisCache) HSet(key, field string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.HSet(ctx, key, field, data).Err()
}

func (r *RedisCache) HGet(key, field string, dest interface{}) error {
	ctx, cancel := r.operationContext()
	defer cancel()
	data, err := r.client.HGet(ctx, key, field).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (r *RedisCache) HDel(key string, fields ...string) error {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.HDel(ctx, key, fields...).Err()
}

func (r *RedisCache) LPush(key string, values ...interface{}) error {
	// 将每个值单独序列化后推入
	for _, value := range values {
		var data []byte
		var err error

		// 如果已经是字符串，直接使用
		if str, ok := value.(string); ok {
			data = []byte(str)
		} else {
			data, err = json.Marshal(value)
			if err != nil {
				return err
			}
		}

		ctx, cancel := r.operationContext()
		err = r.client.LPush(ctx, key, data).Err()
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisCache) RPop(key string) (string, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	result, err := r.client.RPop(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("队列为空")
	}
	return result, err
}

// RPush 将值追加到列表尾部。
func (r *RedisCache) RPush(key string, values ...interface{}) error {
	for _, value := range values {
		var data []byte
		var err error

		if str, ok := value.(string); ok {
			data = []byte(str)
		} else {
			data, err = json.Marshal(value)
			if err != nil {
				return err
			}
		}

		ctx, cancel := r.operationContext()
		err = r.client.RPush(ctx, key, data).Err()
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

// BRPop 阻塞式弹出（带超时）
func (r *RedisCache) BRPop(timeout time.Duration, keys ...string) (string, string, error) {
	ctx, cancel := r.operationContext(timeout)
	defer cancel()
	result, err := r.client.BRPop(ctx, timeout, keys...).Result()
	if err != nil {
		if err == redis.Nil {
			return "", "", fmt.Errorf("队列超时")
		}
		return "", "", err
	}
	if len(result) < 2 {
		return "", "", fmt.Errorf("无效的响应")
	}
	return result[0], result[1], nil
}

// BRPopLPush 原子地从 source 尾部弹出并推入 destination 头部。
func (r *RedisCache) BRPopLPush(source, destination string, timeout time.Duration) (string, error) {
	ctx, cancel := r.operationContext(timeout)
	defer cancel()
	result, err := r.client.BRPopLPush(ctx, source, destination, timeout).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("队列超时")
		}
		return "", err
	}
	return result, nil
}

func (r *RedisCache) LRange(key string, start, stop int64) ([]string, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.LRange(ctx, key, start, stop).Result()
}

func (r *RedisCache) LRem(key string, count int64, value interface{}) (int64, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.LRem(ctx, key, count, value).Result()
}

// ReplaceListItem atomically replaces one list item while preserving reliable-queue invariants.
func (r *RedisCache) ReplaceListItem(key, oldValue, newValue string) (bool, error) {
	const script = `
local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
if removed == 0 then
	return 0
end
redis.call('LPUSH', KEYS[1], ARGV[2])
return 1
`
	ctx, cancel := r.operationContext()
	defer cancel()
	result, err := r.client.Eval(ctx, script, []string{key}, oldValue, newValue).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// MoveListItemToList atomically removes one item from a source list and pushes
// a payload to the destination list. It is used by the reliable queue for ACK
// failure paths without exposing a remove-then-push gap.
func (r *RedisCache) MoveListItemToList(source, destination, oldValue, newValue string) (bool, error) {
	const script = `
local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
if removed == 0 then
	return 0
end
redis.call('LPUSH', KEYS[2], ARGV[2])
return 1
`
	ctx, cancel := r.operationContext()
	defer cancel()
	result, err := r.client.Eval(ctx, script, []string{source, destination}, oldValue, newValue).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// MoveListItemToZSet atomically removes one item from a source list and writes
// a payload to a sorted set with the supplied score.
func (r *RedisCache) MoveListItemToZSet(source, destination, oldValue, newValue string, score float64) (bool, error) {
	const script = `
local removed = redis.call('LREM', KEYS[1], 1, ARGV[1])
if removed == 0 then
	return 0
end
redis.call('ZADD', KEYS[2], ARGV[3], ARGV[2])
return 1
`
	ctx, cancel := r.operationContext()
	defer cancel()
	result, err := r.client.Eval(ctx, script, []string{source, destination}, oldValue, newValue, strconv.FormatFloat(score, 'f', -1, 64)).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// MoveZSetItemToList atomically promotes one sorted-set member into a list.
func (r *RedisCache) MoveZSetItemToList(source, destination, member string) (bool, error) {
	const script = `
local removed = redis.call('ZREM', KEYS[1], ARGV[1])
if removed == 0 then
	return 0
end
redis.call('LPUSH', KEYS[2], ARGV[1])
return 1
`
	ctx, cancel := r.operationContext()
	defer cancel()
	result, err := r.client.Eval(ctx, script, []string{source, destination}, member).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func (r *RedisCache) LLen(key string) int64 {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.LLen(ctx, key).Val()
}

func (r *RedisCache) ZAdd(key string, score float64, member interface{}) error {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.ZAdd(ctx, key, &redis.Z{Score: score, Member: member}).Err()
}

func (r *RedisCache) ZRangeByScore(key string, min, max string, count int64) ([]string, error) {
	opt := &redis.ZRangeBy{
		Min: min,
		Max: max,
	}
	if count > 0 {
		opt.Count = count
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.ZRangeByScore(ctx, key, opt).Result()
}

func (r *RedisCache) ZRem(key string, members ...interface{}) (int64, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.ZRem(ctx, key, members...).Result()
}

func (r *RedisCache) ZCard(key string) int64 {
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.ZCard(ctx, key).Val()
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
func (r *RedisCache) ScanKeysByPrefix(prefix string, count int64) ([]string, error) {
	if count <= 0 {
		count = 100
	}

	pattern := prefix + "*"
	var cursor uint64
	var keys []string

	for {
		ctx, cancel := r.operationContext()
		batch, nextCursor, err := r.client.Scan(ctx, cursor, pattern, count).Result()
		cancel()
		if err != nil {
			return nil, err
		}
		if len(batch) > 0 {
			keys = append(keys, batch...)
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return keys, nil
}

// DelByPrefix 删除指定前缀的所有键，返回删除数量。
func (r *RedisCache) DelByPrefix(prefix string) (int64, error) {
	keys, err := r.ScanKeysByPrefix(prefix, 200)
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, nil
	}
	ctx, cancel := r.operationContext()
	defer cancel()
	return r.client.Del(ctx, keys...).Result()
}

func (r *RedisCache) Close() error {
	if r.cancel != nil {
		r.cancel()
	}
	return r.client.Close()
}

// RateLimitCheck 原子性地检查并递增计数器，返回 (allowed, currentCount, ttl)。
// 若 key 不存在则初始化为 1 并设置过期；若已存在则递增并检查是否超限。
func (r *RedisCache) RateLimitCheck(key string, limit int, window time.Duration) (bool, int64, time.Duration, error) {
	ctx, cancel := r.operationContext()
	defer cancel()

	// Lua 脚本保证原子性：INCR + EXPIRE
	script := redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
if current == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
local ttl = redis.call('TTL', KEYS[1])
return {current, ttl}
`)
	result, err := script.Run(ctx, r.client, []string{key}, int(window.Seconds())).Result()
	if err != nil {
		return false, 0, 0, fmt.Errorf("rate limit script: %w", err)
	}

	vals, ok := result.([]interface{})
	if !ok || len(vals) < 2 {
		return false, 0, 0, fmt.Errorf("unexpected script result")
	}
	count, _ := vals[0].(int64)
	ttl, _ := vals[1].(int64)

	return count <= int64(limit), count, time.Duration(ttl) * time.Second, nil
}
