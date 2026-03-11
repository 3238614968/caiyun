package cache

import (
	"fmt"
	"time"
)

// RedisStorage Redis存储实现（实现tasks.Storage接口）
type RedisStorage struct {
	cache  *RedisCache
	prefix string
}

// NewRedisStorage 创建Redis存储
func NewRedisStorage(cache *RedisCache, prefix string) *RedisStorage {
	return &RedisStorage{
		cache:  cache,
		prefix: prefix,
	}
}

// Get 获取值
func (rs *RedisStorage) Get(key string) (string, error) {
	var value string
	fullKey := rs.getKey(key)
	if err := rs.cache.Get(fullKey, &value); err != nil {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return value, nil
}

// Set 设置值
func (rs *RedisStorage) Set(key, value string) error {
	fullKey := rs.getKey(key)
	// 设置24小时过期时间
	return rs.cache.Set(fullKey, value, 24*time.Hour)
}

// Delete 删除值
func (rs *RedisStorage) Delete(key string) error {
	fullKey := rs.getKey(key)
	return rs.cache.Del(fullKey)
}

// getKey 获取完整键名
func (rs *RedisStorage) getKey(key string) string {
	return fmt.Sprintf("%s:%s", rs.prefix, key)
}
