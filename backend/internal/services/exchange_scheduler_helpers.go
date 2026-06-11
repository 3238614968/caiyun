package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"strconv"
	"time"
)

func scheduledPrepareSlot(now time.Time) (int, int, bool) {
	if constants.ExchangePreInitSeconds <= 0 {
		return 0, 0, false
	}

	executeTime := now.Add(time.Duration(constants.ExchangePreInitSeconds) * time.Second)
	if executeTime.Second() != 0 {
		return 0, 0, false
	}

	return executeTime.Hour(), executeTime.Minute(), true
}

func scheduledExecuteSlot(now time.Time) (int, int, bool) {
	if now.Second() != 0 {
		return 0, 0, false
	}
	return now.Hour(), now.Minute(), true
}

func mergeExchangeTasks(existing []*models.ExchangeTask, incoming []*models.ExchangeTask) []*models.ExchangeTask {
	if len(incoming) == 0 {
		return existing
	}

	seen := make(map[uint]struct{}, len(existing)+len(incoming))
	merged := make([]*models.ExchangeTask, 0, len(existing)+len(incoming))
	for _, task := range existing {
		if task == nil {
			continue
		}
		seen[task.ID] = struct{}{}
		merged = append(merged, task)
	}
	for _, task := range incoming {
		if task == nil {
			continue
		}
		if _, ok := seen[task.ID]; ok {
			continue
		}
		seen[task.ID] = struct{}{}
		merged = append(merged, task)
	}
	return merged
}

func humanizeExchangePeriod(period string) string {
	switch period {
	case "morning":
		return "上午"
	case "evening":
		return "下午"
	default:
		return period
	}
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
func (s *ExchangeScheduler) getConfiguredConcurrency() int {
	if s.configRepo == nil {
		return constants.DefaultConcurrency
	}

	config, err := s.configRepo.GetByKey(constants.ConfigKeyExchangeConcurrency)
	if err != nil || config == nil || config.KeyValue == "" {
		return constants.DefaultConcurrency
	}

	concurrency, err := strconv.Atoi(config.KeyValue)
	if err != nil || concurrency <= 0 {
		return constants.DefaultConcurrency
	}
	if concurrency > 1000 {
		return 1000
	}
	return concurrency
}
