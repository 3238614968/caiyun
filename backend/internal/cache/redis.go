package cache

import (
	"context"
	"encoding/json"
	"fmt"
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

func (r *RedisCache) LLen(key string) int64 {
	return r.client.LLen(r.ctx, key).Val()
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
