package cache

import (
	"caiyun/internal/envutil"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	rdb.AddHook(redisTraceHook{})

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

// ExtendIfValue renews a lease only while the caller still owns it.  A plain
// EXPIRE after a Redis lease handoff could prolong another worker's lock.
func (r *RedisCache) ExtendIfValue(key, value string, expiration time.Duration) (bool, error) {
	if expiration <= 0 {
		return false, fmt.Errorf("lease expiration must be positive")
	}
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
return 0
`
	ctx, cancel := r.operationContext()
	defer cancel()
	renewed, err := r.client.Eval(ctx, script, []string{key}, value, expiration.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return renewed > 0, nil
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

// ServerTime returns the Redis server clock. It is used during startup to
// detect host/container clock drift, which would otherwise invalidate JWTs
// and time-based exchange schedules inconsistently.
func (r *RedisCache) ServerTime() (time.Time, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	serverTime, err := r.client.Time(ctx).Result()
	if err != nil {
		return time.Time{}, err
	}
	return serverTime, nil
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

// IncrementWithTTL atomically increments a counter and applies the supplied
// expiry only when the key is first created. This avoids lost login-failure
// updates under concurrent requests.
func (r *RedisCache) IncrementWithTTL(key string, window time.Duration) (int, error) {
	ctx, cancel := r.operationContext()
	defer cancel()
	script := redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return count
`)
	value, err := script.Run(ctx, r.client, []string{key}, int64(window/time.Second)).Int()
	return value, err
}
