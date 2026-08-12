package services

import (
	"caiyun/internal/models"
	"caiyun/internal/ws"
	"log"
	"strings"
	"time"
)

// reportSkippedTask 记录被调度层跳过的任务结果，避免重复入队时前端完全无感知。
func (s *ExchangeScheduler) reportSkippedTask(task *models.ExchangeTask, message string) {
	if task == nil {
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
func (s *ExchangeScheduler) recordResult(task *models.ExchangeTask, success bool, message string, execTime int) {
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
			"message":      message,
			"execution_ms": execTime,
		},
	})
}

func (s *ExchangeScheduler) finalizeTaskResult(task *models.ExchangeTask, executionToken string, success bool, message string, execTime int) {
	if task == nil {
		return
	}
	if executionToken == "" {
		// This task was skipped before this Worker acquired it.  Only annotate a
		// still-pending task; never reset another Worker's running lease.
		s.reportSkippedTask(task, message)
		return
	}

	status := models.ExchangeTaskPending
	if success {
		if isSingleRunExchangeTask(task.TaskType) && task.AttemptedCount+1 >= task.MaxAttempts {
			status = models.ExchangeTaskCompleted
		}
	} else if s.shouldStopExchange(message) || strings.Contains(message, "商品已下架或不存在") || strings.Contains(message, "商品ID不是可兑换 prizeId") {
		status = models.ExchangeTaskCompleted
	} else if task.RetryCount >= effectiveExchangeMaxRetries(task.MaxRetries) {
		// The scheduled path uses the same retry state machine as an immediate
		// execution. A transient failure that exhausted its configured attempts
		// is terminal rather than silently returning to the next schedule slot.
		status = models.ExchangeTaskFailed
	}

	if err := s.exchangeTaskRepo.FinalizeOwned(task, executionToken, success, message, status, execTime); err != nil {
		log.Printf("【抢兑调度器】保存任务结果失败: task_id=%d err=%v", task.ID, err)
		return
	}
	task.AttemptedCount++
	task.LastResult = message
	task.Status = string(status)
	if success {
		task.SuccessCount++
	} else {
		task.FailCount++
	}
	s.recordResult(task, success, message, execTime)
}
