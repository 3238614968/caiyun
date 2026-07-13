package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"fmt"
	"strconv"
	"strings"
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

func nextSchedulerWakeDelay(now time.Time) time.Duration {
	triggerSeconds := schedulerTriggerSeconds()
	best := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), triggerSeconds[0], 0, now.Location())
	if !best.After(now) {
		best = best.Add(time.Minute)
	}
	for _, second := range triggerSeconds[1:] {
		candidate := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), second, 0, now.Location())
		if !candidate.After(now) {
			candidate = candidate.Add(time.Minute)
		}
		if candidate.Before(best) {
			best = candidate
		}
	}
	delay := best.Sub(now)
	if delay <= 0 {
		return time.Second
	}
	return delay
}

func schedulerTriggerSeconds() []int {
	seconds := []int{0}
	if constants.ExchangePreInitSeconds > 0 {
		prepareSecond := (60 - (constants.ExchangePreInitSeconds % 60)) % 60
		if prepareSecond != 0 {
			seconds = append(seconds, prepareSecond)
		}
	}
	if len(seconds) == 2 && seconds[1] < seconds[0] {
		seconds[0], seconds[1] = seconds[1], seconds[0]
	}
	return seconds
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

func normalizeExchangeSlotTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if parsed, err := time.Parse("15:04:05", value); err == nil {
		return parsed.Format("15:04:05")
	}
	if parsed, err := time.Parse("15:04", value); err == nil {
		return parsed.Format("15:04:05")
	}
	return value
}

func taskMatchesExchangeSlot(task *models.ExchangeTask, hour, minute int, now time.Time) bool {
	if task == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	slot := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if ok, _ := repository.ShouldRunExchangeTaskAt(task, slot); ok {
		return true
	}
	timeStr := fmt.Sprintf("%02d:%02d:00", hour, minute)
	if scheduled := normalizeExchangeSlotTime(task.ScheduledExchangeTime); scheduled != "" {
		return scheduled == timeStr
	}
	return normalizeExchangeSlotTime(task.ExchangeAccount.ExchangeTime1) == timeStr || normalizeExchangeSlotTime(task.ExchangeAccount.ExchangeTime2) == timeStr
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
