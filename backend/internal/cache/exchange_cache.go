package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ExchangeCache 兑换中心缓存管理器
type ExchangeCache struct {
	redis *RedisCache
}

// NewExchangeCache 创建兑换中心缓存管理器
func NewExchangeCache(redis *RedisCache) *ExchangeCache {
	return &ExchangeCache{redis: redis}
}

// 缓存键前缀
const (
	CacheKeyProduct    = "exchange:product:"
	CacheKeyProducts   = "exchange:products:list"
	CacheKeyCategories = "exchange:categories"
	CacheKeyAccount    = "exchange:account:"
	CacheKeyTask       = "exchange:task:"
	CacheKeyConfig     = "exchange:config:"
	CacheKeyStats      = "exchange:stats:"
)

// 默认过期时间
const (
	ProductTTL = 10 * time.Minute // 商品缓存 10 分钟
	AccountTTL = 30 * time.Minute // 账号缓存 30 分钟
	TaskTTL    = 5 * time.Minute  // 任务缓存 5 分钟
	ConfigTTL  = 60 * time.Minute // 配置缓存 1 小时
	StatsTTL   = 5 * time.Minute  // 统计缓存 5 分钟
)

// CacheProduct 缓存商品信息
func (c *ExchangeCache) CacheProduct(prizeID string, product interface{}) error {
	key := fmt.Sprintf("%s%s", CacheKeyProduct, prizeID)
	data, err := json.Marshal(product)
	if err != nil {
		return fmt.Errorf("序列化商品失败：%w", err)
	}

	if err := c.redis.Set(key, string(data), ProductTTL); err != nil {
		return fmt.Errorf("缓存商品失败：%w", err)
	}

	return nil
}

// GetProduct 获取缓存的商品
func (c *ExchangeCache) GetProduct(prizeID string) (interface{}, error) {
	key := fmt.Sprintf("%s%s", CacheKeyProduct, prizeID)
	var product interface{}
	err := c.redis.Get(key, &product)
	if err != nil {
		return nil, err
	}

	return product, nil
}

// DeleteProduct 删除商品缓存
func (c *ExchangeCache) DeleteProduct(prizeID string) error {
	key := fmt.Sprintf("%s%s", CacheKeyProduct, prizeID)
	return c.redis.Del(key)
}

// CacheProducts 缓存商品列表
func (c *ExchangeCache) CacheProducts(products interface{}) error {
	data, err := json.Marshal(products)
	if err != nil {
		return fmt.Errorf("序列化商品列表失败：%w", err)
	}

	if err := c.redis.Set(CacheKeyProducts, string(data), ProductTTL); err != nil {
		return fmt.Errorf("缓存商品列表失败：%w", err)
	}

	return nil
}

// GetProducts 获取缓存的商品列表
func (c *ExchangeCache) GetProducts() (interface{}, error) {
	var products interface{}
	err := c.redis.Get(CacheKeyProducts, &products)
	if err != nil {
		return nil, err
	}

	return products, nil
}

// DeleteProducts 删除商品列表缓存
func (c *ExchangeCache) DeleteProducts() error {
	return c.redis.Del(CacheKeyProducts)
}

// CacheCategories 缓存分类列表
func (c *ExchangeCache) CacheCategories(categories []string) error {
	data, err := json.Marshal(categories)
	if err != nil {
		return fmt.Errorf("序列化分类失败：%w", err)
	}

	if err := c.redis.Set(CacheKeyCategories, string(data), ProductTTL); err != nil {
		return fmt.Errorf("缓存分类失败：%w", err)
	}

	return nil
}

// GetCategories 获取缓存的分类
func (c *ExchangeCache) GetCategories() ([]string, error) {
	var categories []string
	err := c.redis.Get(CacheKeyCategories, &categories)
	if err != nil {
		return nil, err
	}

	return categories, nil
}

// CacheAccount 缓存兑换账号信息
func (c *ExchangeCache) CacheAccount(accountID uint, account interface{}) error {
	key := fmt.Sprintf("%s%d", CacheKeyAccount, accountID)
	data, err := json.Marshal(account)
	if err != nil {
		return fmt.Errorf("序列化账号失败：%w", err)
	}

	if err := c.redis.Set(key, string(data), AccountTTL); err != nil {
		return fmt.Errorf("缓存账号失败：%w", err)
	}

	return nil
}

// GetAccount 获取缓存的账号
func (c *ExchangeCache) GetAccount(accountID uint) (interface{}, error) {
	key := fmt.Sprintf("%s%d", CacheKeyAccount, accountID)
	var account interface{}
	err := c.redis.Get(key, &account)
	if err != nil {
		return nil, err
	}

	return account, nil
}

// DeleteAccount 删除账号缓存
func (c *ExchangeCache) DeleteAccount(accountID uint) error {
	key := fmt.Sprintf("%s%d", CacheKeyAccount, accountID)
	return c.redis.Del(key)
}

// InvalidateAll 清空所有兑换中心缓存
func (c *ExchangeCache) InvalidateAll() error {
	prefixes := []string{
		CacheKeyProduct,
		CacheKeyProducts,
		CacheKeyCategories,
		CacheKeyAccount,
		CacheKeyTask,
		CacheKeyConfig,
		CacheKeyStats,
	}

	for _, prefix := range prefixes {
		if _, err := c.redis.DelByPrefix(prefix); err != nil {
			return fmt.Errorf("按前缀清理缓存失败（%s）: %w", prefix, err)
		}
	}

	return nil
}

// WarmUpCache 预热缓存（启动时加载常用数据）
func (c *ExchangeCache) WarmUpCache(ctx context.Context) error {
	// 这里可以添加逻辑从数据库加载热门商品、活跃账号等数据到缓存
	// 示例：预加载商品列表和分类
	return nil
}
