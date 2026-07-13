package services

import (
	"fmt"
	"strconv"
	"sync"

	"caiyun/internal/envutil"
	"caiyun/internal/repository"
)

// ConfigService provides database-over-environment business configuration.
// Environment parsing is centralized in envutil; startup configuration remains owned by bootstrap.
type ConfigService struct {
	configRepo *repository.SystemConfigRepository
	mu         sync.RWMutex
	cache      map[string]interface{}
}

func NewConfigService(configRepo *repository.SystemConfigRepository) *ConfigService {
	return &ConfigService{configRepo: configRepo, cache: make(map[string]interface{})}
}

func (s *ConfigService) GetInt(key string, fallback int) int {
	s.mu.RLock()
	value, ok := s.cache[key]
	s.mu.RUnlock()
	if ok {
		if typed, valid := value.(int); valid {
			return typed
		}
	}
	if s.configRepo != nil {
		if cfg, err := s.configRepo.GetByKey(key); err == nil && cfg.KeyValue != "" {
			if parsed, parseErr := strconv.Atoi(cfg.KeyValue); parseErr == nil {
				s.setCached(key, parsed)
				return parsed
			}
		}
	}
	return envutil.Int(key, fallback)
}

func (s *ConfigService) GetString(key, fallback string) string {
	s.mu.RLock()
	value, ok := s.cache[key]
	s.mu.RUnlock()
	if ok {
		if typed, valid := value.(string); valid {
			return typed
		}
	}
	if s.configRepo != nil {
		if cfg, err := s.configRepo.GetByKey(key); err == nil && cfg.KeyValue != "" {
			s.setCached(key, cfg.KeyValue)
			return cfg.KeyValue
		}
	}
	return envutil.String(key, fallback)
}

func (s *ConfigService) GetBool(key string, fallback bool) bool {
	s.mu.RLock()
	value, ok := s.cache[key]
	s.mu.RUnlock()
	if ok {
		if typed, valid := value.(bool); valid {
			return typed
		}
	}
	if s.configRepo != nil {
		if cfg, err := s.configRepo.GetByKey(key); err == nil && cfg.KeyValue != "" {
			parsed := cfg.KeyValue == "true" || cfg.KeyValue == "1" || cfg.KeyValue == "yes"
			s.setCached(key, parsed)
			return parsed
		}
	}
	return envutil.Bool(key, fallback)
}

func (s *ConfigService) SetInt(key string, value int, description string) error {
	return s.set(key, strconv.Itoa(value), description)
}
func (s *ConfigService) SetString(key, value, description string) error {
	return s.set(key, value, description)
}
func (s *ConfigService) SetBool(key string, value bool, description string) error {
	return s.set(key, strconv.FormatBool(value), description)
}
func (s *ConfigService) set(key, value, description string) error {
	s.InvalidateCache(key)
	if s.configRepo == nil {
		return fmt.Errorf("配置仓库未初始化")
	}
	return s.configRepo.UpdateByKey(key, value, description)
}
func (s *ConfigService) setCached(key string, value interface{}) {
	s.mu.Lock()
	s.cache[key] = value
	s.mu.Unlock()
}
func (s *ConfigService) InvalidateCache(key string) { s.mu.Lock(); delete(s.cache, key); s.mu.Unlock() }
func (s *ConfigService) ClearCache() {
	s.mu.Lock()
	s.cache = make(map[string]interface{})
	s.mu.Unlock()
}
func (s *ConfigService) ReloadFromDB() error {
	if s.configRepo == nil {
		return fmt.Errorf("配置仓库未初始化")
	}
	configs, err := s.configRepo.GetAll()
	if err != nil {
		return err
	}
	fresh := make(map[string]interface{}, len(configs))
	for _, cfg := range configs {
		fresh[cfg.KeyName] = cfg.KeyValue
	}
	s.mu.Lock()
	s.cache = fresh
	s.mu.Unlock()
	return nil
}
