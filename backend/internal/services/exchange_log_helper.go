package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"fmt"
	"log"
	"strings"
)

func exchangeAccountName(account *models.ExchangeAccount) string {
	if account == nil {
		return ""
	}
	if account.Remark != "" {
		return account.Remark
	}
	return account.Phone
}

func isSingleRunExchangeTask(taskType string) bool {
	return taskType == string(models.ExchangeTaskFixed) || taskType == "immediate"
}

func createExchangeSystemLog(taskLogRepo *repository.TaskLogRepository, userID, accountID uint, prizeName, accountName string, success bool, message string, execTimeMs int) {
	if taskLogRepo == nil || accountID == 0 {
		return
	}

	status := "failed"
	if success {
		status = "success"
	}

	parts := make([]string, 0, 4)
	if prizeName != "" {
		parts = append(parts, fmt.Sprintf("商品: %s", prizeName))
	}
	if accountName != "" {
		parts = append(parts, fmt.Sprintf("兑换账号: %s", accountName))
	}
	if message != "" {
		parts = append(parts, fmt.Sprintf("结果: %s", message))
	}

	entry := &models.TaskLog{
		UserID:        userID,
		AccountID:     accountID,
		TaskType:      "exchange",
		Status:        status,
		Message:       strings.Join(parts, " | "),
		CloudGained:   0,
		ExecutionTime: execTimeMs,
	}

	if err := taskLogRepo.Create(entry); err != nil {
		log.Printf("【抢兑任务】写入系统日志失败: %v", err)
	}
}
