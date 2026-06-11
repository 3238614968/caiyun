package cache

import (
	"context"
	"encoding/json"
	"fmt"
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
	client *redis.Client
	ctx    context.Context
}

type StreamMessage struct {
	ID     string
	Values map[string]interface{}
}

func NewRedisClient(addr, password string, db int) (*RedisCache, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     50,
		MinIdleConns: 10,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
		IdleTimeout:  5 * time.Minute,
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("Redis连接失败: %w", err)
	}

	return &RedisCache{
		client: rdb,
		ctx:    ctx,
	}, nil
}

func (r *RedisCache) Set(key string, value interface{}, expiration time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.Set(r.ctx, key, data, expiration).Err()
}

func (r *RedisCache) SetNX(key string, value interface{}, expiration time.Duration) (bool, error) {
	return r.client.SetNX(r.ctx, key, value, expiration).Result()
}

func (r *RedisCache) DelIfValue(key, value string) (bool, error) {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	deleted, err := r.client.Eval(r.ctx, script, []string{key}, value).Int()
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func (r *RedisCache) Get(key string, dest interface{}) error {
	data, err := r.client.Get(r.ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (r *RedisCache) Del(keys ...string) error {
	return r.client.Del(r.ctx, keys...).Err()
}

func (r *RedisCache) Exists(keys ...string) (int64, error) {
	return r.client.Exists(r.ctx, keys...).Result()
}

func (r *RedisCache) HSet(key, field string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return r.client.HSet(r.ctx, key, field, data).Err()
}

func (r *RedisCache) HGet(key, field string, dest interface{}) error {
	data, err := r.client.HGet(r.ctx, key, field).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func (r *RedisCache) HDel(key string, fields ...string) error {
	return r.client.HDel(r.ctx, key, fields...).Err()
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

		if err := r.client.LPush(r.ctx, key, data).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (r *RedisCache) RPop(key string) (string, error) {
	result, err := r.client.RPop(r.ctx, key).Result()
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

		if err := r.client.RPush(r.ctx, key, data).Err(); err != nil {
			return err
		}
	}
	return nil
}

// BRPop 阻塞式弹出（带超时）
func (r *RedisCache) BRPop(timeout time.Duration, keys ...string) (string, string, error) {
	result, err := r.client.BRPop(r.ctx, timeout, keys...).Result()
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
	result, err := r.client.BRPopLPush(r.ctx, source, destination, timeout).Result()
	if err != nil {
		if err == redis.Nil {
			return "", fmt.Errorf("队列超时")
		}
		return "", err
	}
	return result, nil
}

func (r *RedisCache) LRange(key string, start, stop int64) ([]string, error) {
	return r.client.LRange(r.ctx, key, start, stop).Result()
}

func (r *RedisCache) LRem(key string, count int64, value interface{}) (int64, error) {
	return r.client.LRem(r.ctx, key, count, value).Result()
}

func (r *RedisCache) LLen(key string) int64 {
	return r.client.LLen(r.ctx, key).Val()
}

func (r *RedisCache) ZAdd(key string, score float64, member interface{}) error {
	return r.client.ZAdd(r.ctx, key, &redis.Z{Score: score, Member: member}).Err()
}

func (r *RedisCache) ZRangeByScore(key string, min, max string, count int64) ([]string, error) {
	opt := &redis.ZRangeBy{
		Min: min,
		Max: max,
	}
	if count > 0 {
		opt.Count = count
	}
	return r.client.ZRangeByScore(r.ctx, key, opt).Result()
}

func (r *RedisCache) ZRem(key string, members ...interface{}) (int64, error) {
	return r.client.ZRem(r.ctx, key, members...).Result()
}

func (r *RedisCache) ZCard(key string) int64 {
	return r.client.ZCard(r.ctx, key).Val()
}

func (r *RedisCache) XGroupCreateMkStream(stream, group, start string) error {
	if start == "" {
		start = "0"
	}
	err := r.client.XGroupCreateMkStream(r.ctx, stream, group, start).Err()
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
	return r.client.XAdd(r.ctx, args).Result()
}

func (r *RedisCache) XReadGroup(group, consumer, stream, id string, count int64, block time.Duration) ([]StreamMessage, error) {
	if id == "" {
		id = ">"
	}
	if count <= 0 {
		count = 1
	}
	result, err := r.client.XReadGroup(r.ctx, &redis.XReadGroupArgs{
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
	return r.client.XAck(r.ctx, stream, group, ids...).Result()
}

func (r *RedisCache) XDel(stream string, ids ...string) (int64, error) {
	return r.client.XDel(r.ctx, stream, ids...).Result()
}

func (r *RedisCache) XAutoClaim(stream, group, consumer string, minIdle time.Duration, start string, count int64) ([]StreamMessage, string, error) {
	if start == "" {
		start = "0-0"
	}
	if count <= 0 {
		count = 100
	}
	messages, nextStart, err := r.client.XAutoClaim(r.ctx, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		Consumer: consumer,
		MinIdle:  minIdle,
		Start:    start,
		Count:    count,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nextStart, nil
		}
		return nil, nextStart, err
	}
	return flattenSingleStreamMessages(messages), nextStart, nil
}

func (r *RedisCache) XPendingCount(stream, group string) (int64, error) {
	pending, err := r.client.XPending(r.ctx, stream, group).Result()
	if err != nil {
		if err == redis.Nil || strings.Contains(err.Error(), "NOGROUP") {
			return 0, nil
		}
		return 0, err
	}
	return pending.Count, nil
}

func (r *RedisCache) XLen(stream string) int64 {
	return r.client.XLen(r.ctx, stream).Val()
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

// ScanKeysByPrefix 按前缀扫描 Redis 键，避免使用阻塞式 KEYS 命令。
func (r *RedisCache) ScanKeysByPrefix(prefix string, count int64) ([]string, error) {
	if count <= 0 {
		count = 100
	}

	pattern := prefix + "*"
	var cursor uint64
	var keys []string

	for {
		batch, nextCursor, err := r.client.Scan(r.ctx, cursor, pattern, count).Result()
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
	return r.client.Del(r.ctx, keys...).Result()
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}
