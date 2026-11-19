package services

import (
	"caiyun/internal/models"
	"strings"
)

// exchangeTaskFinalStatus keeps a long-term task scheduled after a failed
// slot, while preserving manual review for uncertain or invalid results.
func exchangeTaskFinalStatus(task *models.ExchangeTask, result exchangeAttemptResult) models.ExchangeTaskStatus {
	if result.outcome == models.ExchangeOutcomeUnknown {
		return models.ExchangeTaskFailed
	}
	message := result.message
	success := result.success
	retriesUsed := 0
	if task != nil {
		retriesUsed = task.RetryCount
	}
	if !success {
		if strings.Contains(message, "兑换结果待确认") || strings.Contains(message, "任务执行异常") {
			return models.ExchangeTaskFailed
		}
		if strings.Contains(message, "商品已下架或不存在") || strings.Contains(message, "商品ID不是可兑换 prizeId") {
			return models.ExchangeTaskCompleted
		}
	}
	if task.TaskType == string(models.ExchangeTaskLongTerm) {
		return models.ExchangeTaskPending
	}
	exhausted := task.MaxAttempts <= 1 || task.AttemptedCount+1 >= task.MaxAttempts
	if success && exhausted {
		return models.ExchangeTaskCompleted
	}
	if !success && (exhausted || retriesUsed >= effectiveExchangeMaxRetries(task.MaxRetries)) {
		return models.ExchangeTaskFailed
	}
	return models.ExchangeTaskPending
}
