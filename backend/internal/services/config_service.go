package services

import (
	"caiyun/internal/repository"
	"caiyun/pkg/config"
	"fmt"
	"strconv"
	"sync"
)

// ConfigService 配置服务（支持 env 和数据库）
type ConfigService struct {
	configRepo *repository.SystemConfigRepository
	cfg        *config.Config
	mu         sync.RWMutex
	cache      map[string]interface{}
}

// NewConfigService 创建配置服务
func NewConfigService(configRepo *repository.SystemConfigRepository) *ConfigService {
	return &ConfigService{
		configRepo: configRepo,
		cfg:        config.Global(),
		cache:      make(map[string]interface{}),
	}
}

// GetInt 获取整数配置（优先数据库，其次 env，最后默认值）
func (s *ConfigService) GetInt(key string, defaultVal int) int {
	s.mu.RLock()
	if val, ok := s.cache[key]; ok {
		s.mu.RUnlock()
		if v, ok := val.(int); ok {
			return v
		}
	}
	s.mu.RUnlock()

	// 尝试从数据库读取
	if s.configRepo != nil {
		dbConfig, err := s.configRepo.GetByKey(key)
		if err == nil && dbConfig.KeyValue != "" {
			if val, err := strconv.Atoi(dbConfig.KeyValue); err == nil {
				s.mu.Lock()
				s.cache[key] = val
				s.mu.Unlock()
				return val
			}
		}
	}

	// 从环境变量读取（通过 config 包）
	switch key {
	case "TASK_CONCURRENCY":
		return s.cfg.Task.Concurrency
	case "EXCHANGE_CONCURRENCY":
		return s.cfg.Exchange.Concurrency
	case "EXCHANGE_MAX_GLOBAL_CONCURRENCY":
		return s.cfg.Exchange.MaxGlobalConcurrency
	case "EXCHANGE_DEFAULT_TIMEOUT":
		return s.cfg.Exchange.DefaultTimeout
	}

	// 返回默认值
	return defaultVal
}

// GetString 获取字符串配置
func (s *ConfigService) GetString(key, defaultVal string) string {
	s.mu.RLock()
	if val, ok := s.cache[key]; ok {
		s.mu.RUnlock()
		if v, ok := val.(string); ok {
			return v
		}
	}
	s.mu.RUnlock()

	// 尝试从数据库读取
	if s.configRepo != nil {
		dbConfig, err := s.configRepo.GetByKey(key)
		if err == nil && dbConfig.KeyValue != "" {
			s.mu.Lock()
			s.cache[key] = dbConfig.KeyValue
			s.mu.Unlock()
			return dbConfig.KeyValue
		}
	}

	// 从环境变量读取
	switch key {
	case "JWT_SECRET":
		return s.cfg.JWT.Secret
	case "REDIS_HOST":
		return s.cfg.Redis.Host
	case "REDIS_PORT":
		return s.cfg.Redis.Port
	case "EXCHANGE_SCHEDULE_TIME_1":
		return s.cfg.Exchange.ScheduleTime1
	case "EXCHANGE_SCHEDULE_TIME_2":
		return s.cfg.Exchange.ScheduleTime2
	case "EXCHANGE_UPDATE_TIME":
		return s.cfg.Exchange.UpdateTime
	}

	return defaultVal
}

// GetBool 获取布尔配置
func (s *ConfigService) GetBool(key string, defaultVal bool) bool {
	s.mu.RLock()
	if val, ok := s.cache[key]; ok {
		s.mu.RUnlock()
		if v, ok := val.(bool); ok {
			return v
		}
	}
	s.mu.RUnlock()

	// 尝试从数据库读取
	if s.configRepo != nil {
		dbConfig, err := s.configRepo.GetByKey(key)
		if err == nil && dbConfig.KeyValue != "" {
			val := dbConfig.KeyValue == "true" || dbConfig.KeyValue == "1" || dbConfig.KeyValue == "yes"
			s.mu.Lock()
			s.cache[key] = val
			s.mu.Unlock()
			return val
		}
	}

	// 从环境变量读取
	switch key {
	case "EXCHANGE_AUTO_UPDATE_PRODUCTS":
		return s.cfg.Exchange.AutoUpdateProducts
	case "EXCHANGE_ENABLE_PRIORITY":
		return s.cfg.Exchange.EnablePriority
	case "EXCHANGE_AUTO_RETRY_FAILED":
		return s.cfg.Exchange.AutoRetryFailed
	}

	return defaultVal
}

// SetInt 设置整数配置到数据库
func (s *ConfigService) SetInt(key string, value int, description string) error {
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()

	if s.configRepo == nil {
		return fmt.Errorf("配置仓库未初始化")
	}

	return s.configRepo.UpdateByKey(key, fmt.Sprintf("%d", value), description)
}

// SetString 设置字符串配置到数据库
func (s *ConfigService) SetString(key, value, description string) error {
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()

	if s.configRepo == nil {
		return fmt.Errorf("配置仓库未初始化")
	}

	return s.configRepo.UpdateByKey(key, value, description)
}

// SetBool 设置布尔配置到数据库
func (s *ConfigService) SetBool(key string, value bool, description string) error {
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()

	if s.configRepo == nil {
		return fmt.Errorf("配置仓库未初始化")
	}

	strVal := "false"
	if value {
		strVal = "true"
	}
	return s.configRepo.UpdateByKey(key, strVal, description)
}

// InvalidateCache 使缓存失效
func (s *ConfigService) InvalidateCache(key string) {
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()
}

// ClearCache 清空所有缓存
func (s *ConfigService) ClearCache() {
	s.mu.Lock()
	s.cache = make(map[string]interface{})
	s.mu.Unlock()
}

// ReloadFromDB 从数据库重新加载配置到缓存
func (s *ConfigService) ReloadFromDB() error {
	if s.configRepo == nil {
		return fmt.Errorf("配置仓库未初始化")
	}

	configs, err := s.configRepo.GetAll()
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, cfg := range configs {
		s.cache[cfg.KeyName] = cfg.KeyValue
	}

	return nil
}
