package services

import (
	"caiyun/internal/models"
	"caiyun/internal/ws"
	"context"
	"strings"
	"time"
)

// reportSkippedTask 记录被调度层跳过的任务结果，避免重复入队时前端完全无感知。
func (s *ExchangeScheduler) reportSkippedTask(task *models.ExchangeTask, message string) {
	if task == nil || task.Status != string(models.ExchangeTaskPending) {
		return
	}
	_, _ = s.exchangeTaskRepo.UpdatePendingLastResult(task.ID, message)
	createExchangeSystemLog(
		s.taskLogRepo,
		task.UserID,
		task.ExchangeAccount.AccountID,
		task.PrizeName,
		exchangeAccountName(&task.ExchangeAccount),
		false,
		message,
		0,
	)
	s.sendToUser(task.UserID, ws.Message{
		Type: "exchange_result",
		Data: map[string]interface{}{
			"task_id":      task.ID,
			"prize_name":   task.PrizeName,
			"success":      false,
			"message":      message,
			"execution_ms": 0,
		},
	})
}

func (s *ExchangeScheduler) shouldStopExchange(message string) bool {
	// 以下商品级库存/上下架状态应该停止当前商品后续账号抢兑。
	// 账号级结果（如当前账号已兑换、云朵不足）不停止其他账号。
	stopPatterns := []string{
		"无库存",
		"库存不足",
		"已兑完",
		"已耗尽",
		"已下架",
		"奖品单日已耗尽",
		"奖品已兑完",
	}

	for _, pattern := range stopPatterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}

	return false
}

// recordResult 记录抢兑结果，并与手动执行路径保持一致地更新尝试次数和最后结果。
func (s *ExchangeScheduler) recordResult(task *models.ExchangeTask, result exchangeAttemptResult, execTime int) {
	success := result.success
	message := result.message
	if s.metrics != nil {
		s.metrics.RecordExchangeAttempt(success, exchangeFailureReasonLabel(message), time.Duration(execTime)*time.Millisecond)
	}
	createExchangeSystemLog(
		s.taskLogRepo,
		task.UserID,
		task.ExchangeAccount.AccountID,
		task.PrizeName,
		exchangeAccountName(&task.ExchangeAccount),
		success,
		message,
		execTime,
	)

	// 发送WebSocket通知
	s.sendToUser(task.UserID, ws.Message{
		Type: "exchange_result",
		Data: map[string]interface{}{
			"task_id":      task.ID,
			"prize_name":   task.PrizeName,
			"success":      success,
			"outcome":      string(result.outcome),
			"message":      message,
			"execution_ms": execTime,
		},
	})
}

func (s *ExchangeScheduler) finalizeTaskResult(task *models.ExchangeTask, execution exchangeTaskExecutionResult) error {
	if task == nil {
		return nil
	}
	if execution.skipped || task.Status == string(models.ExchangeTaskCompleted) || task.Status == string(models.ExchangeTaskFailed) {
		s.reportSkippedTask(task, execution.message)
		return nil
	}
	return s.persistSchedulerResult(context.Background(), task, execution)
}
