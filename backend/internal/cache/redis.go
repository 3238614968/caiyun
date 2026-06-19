package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func redisDurationFromEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(raw); err == nil && duration > 0 {
		return duration
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
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
