package presentation

import "strings"

// Task logs are written after execution. Their pending/legacy running state is
// an unconfirmed outcome, never evidence of a currently running worker.
func TaskLogStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success", "succeeded", "completed":
		return "success"
	case "failed", "failure":
		return "failed"
	case "skipped", "canceled", "cancelled":
		return "skipped"
	default:
		return "pending"
	}
}

func TaskLogMessage(taskType, status, raw string) string {
	message := Message(taskType, raw)
	if TaskLogStatus(status) == "pending" {
		switch message {
		case "执行中", "正在执行", "等待执行":
			return "执行已结束，结果待确认"
		}
	}
	return message
}
