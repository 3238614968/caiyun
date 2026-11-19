package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"caiyun/internal/models"
)

const taskBatchSummaryPrefix = "task_batch:"

type taskBatchPublicResult struct {
	Success int      `json:"success"`
	Failed  int      `json:"failed"`
	Pending int      `json:"pending"`
	Tasks   []string `json:"tasks"`
}

var failedTaskCode = regexp.MustCompile(`(?:^|[、:：]\s*)([a-z][a-z0-9_]*)\(`)

func publicFailedTaskNames(raw string) []string {
	var names []string
	seen := map[string]bool{}
	for _, match := range failedTaskCode.FindAllStringSubmatch(raw, -1) {
		if definition, ok := defaultTaskCatalog.Get(match[1]); ok && !seen[definition.Code] {
			seen[definition.Code] = true
			names = append(names, definition.Name)
		}
	}
	return names
}

func taskBatchStoredSummary(failure *TaskBatchFailure) string {
	names := publicFailedTaskNames(strings.Join(failure.Tasks, "、"))
	if len(names) > 3 {
		names = names[:3]
	}
	encoded, _ := json.Marshal(taskBatchPublicResult{Success: failure.Summary.Success, Failed: failure.Summary.Failed, Pending: failure.Summary.Pending, Tasks: names})
	return taskBatchSummaryPrefix + string(encoded)
}

// PublicOperationError only exposes summaries generated from known fields.
// Arbitrary upstream errors remain server-side, rather than becoming UI text.
func PublicOperationError(operation *models.Operation) string {
	if operation == nil || operation.Status != models.OperationFailed {
		return ""
	}
	raw := operation.ErrorSummary
	if strings.Contains(raw, ErrExchangeOutcomeUnknown.Error()) {
		return "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
	}
	if strings.HasPrefix(raw, taskBatchSummaryPrefix) {
		var summary taskBatchPublicResult
		if json.Unmarshal([]byte(strings.TrimPrefix(raw, taskBatchSummaryPrefix)), &summary) == nil && summary.Success >= 0 && summary.Failed > 0 && summary.Pending >= 0 {
			prefix := "任务执行失败"
			if summary.Success > 0 {
				prefix = "部分任务未完成"
			}
			return fmt.Sprintf("%s：成功 %d 项，失败 %d 项，待确认 %d 项；请查看任务明细", prefix, summary.Success, summary.Failed, summary.Pending)
		}
	}
	if strings.HasPrefix(raw, "任务批次存在失败项:") {
		if names := publicFailedTaskNames(raw); len(names) > 0 {
			if len(names) > 3 {
				names = append(names[:3], "等")
			}
			return "未完成任务：" + strings.Join(names, "、") + "；请查看任务明细"
		}
	}
	if strings.Contains(raw, "云朵数不足") || strings.Contains(raw, "云朵不足") {
		return "云朵不足，兑换未完成"
	}
	return "操作未完成，请查看任务明细"
}
