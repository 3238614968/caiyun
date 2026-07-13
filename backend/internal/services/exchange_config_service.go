package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"fmt"
	"strconv"
)

// GetExchangeConcurrency 获取抢兑并发数
func (s *ExchangeService) GetExchangeConcurrency() (int, error) {
	config, err := s.configRepo.GetByKey("exchange_concurrency")
	if err != nil {
		return constants.DefaultConcurrency, nil // 使用常量：默认 10 并发
	}

	// 使用 strconv 代替 fmt.Sscanf，避免乱码问题
	if config.KeyValue == "" {
		return 10, nil
	}

	concurrency, err := strconv.Atoi(config.KeyValue)
	if err != nil {
		// 解析失败返回默认值
		return 10, nil
	}

	// 验证范围
	if concurrency <= 0 {
		return 10, nil
	}
	if concurrency > 1000 {
		concurrency = 1000 // 限制最大值
	}

	return concurrency, nil
}

// SetExchangeConcurrency 设置抢兑并发数
func (s *ExchangeService) SetExchangeConcurrency(concurrency int) error {
	// 验证参数
	if concurrency <= 0 {
		return fmt.Errorf("并发数必须大于 0")
	}
	if concurrency > 1000 {
		return fmt.Errorf("并发数不能超过 1000")
	}

	return s.configRepo.UpdateByKey("exchange_concurrency", fmt.Sprintf("%d", concurrency), "抢兑任务并发数量")
}

// GetSystemConfig 获取系统配置
func (s *ExchangeService) GetSystemConfig(key string) (*models.SystemConfig, error) {
	return s.configRepo.GetByKey(key)
}

// SystemConfigUpdate 表示一次系统配置更新请求。
type SystemConfigUpdate struct {
	Key         string
	Value       string
	Description string
}

// SetSystemConfig 设置系统配置
func (s *ExchangeService) SetSystemConfig(key, value, description string) error {
	return s.configRepo.UpdateByKey(key, value, description)
}

// SetSystemConfigs 在同一事务中设置多项系统配置。
func (s *ExchangeService) SetSystemConfigs(updates []SystemConfigUpdate) error {
	repoUpdates := make([]repository.SystemConfigUpdate, 0, len(updates))
	for _, update := range updates {
		repoUpdates = append(repoUpdates, repository.SystemConfigUpdate{
			Key:         update.Key,
			Value:       update.Value,
			Description: update.Description,
		})
	}
	return s.configRepo.BatchUpdate(repoUpdates)
}
