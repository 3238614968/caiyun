package services

import (
	"fmt"
	"strings"
)

// A persisted batch failure must not replay its successful mutations.
type TaskBatchFailure struct{ Tasks []string }

func (e *TaskBatchFailure) Error() string {
	return "任务批次存在失败项: " + strings.Join(e.Tasks, "、") + "；可单独重试失败任务"
}
func (e *TaskBatchFailure) Retryable() bool { return false }

func taskBatchOutcome(results []TaskResult) error {
	var failed []string
	for _, r := range results {
		if r.Status == "failed" {
			failed = append(failed, fmt.Sprintf("%s(%s)", r.TaskType, r.Message))
		}
	}
	if len(failed) > 0 {
		return &TaskBatchFailure{Tasks: failed}
	}
	return nil
}

func orderTaskCompletionPhases(codes []string) []string {
	var work, receive, stats, cleanup []string
	for _, code := range codes {
		switch defaultTaskCatalog.Normalize(code) {
		case "receive":
			receive = append(receive, code)
		case "todaycloud":
			stats = append(stats, code)
		case "after_task":
			cleanup = append(cleanup, code)
		default:
			work = append(work, code)
		}
	}
	return append(append(append(work, receive...), stats...), cleanup...)
}

func remainingBatchTaskCodes(codes, done []string) []string {
	completed := make(map[string]bool)
	for _, code := range done {
		completed[code] = true
	}
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		normalized := defaultTaskCatalog.Normalize(code)
		switch normalized {
		case "receive", "todaycloud", "after_task", "prize_center", "hidden_rewards", "tasklist":
			out = append(out, code)
		default:
			if !completed[normalized] {
				out = append(out, code)
			}
		}
	}
	return out
}
