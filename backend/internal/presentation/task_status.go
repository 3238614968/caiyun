package presentation

import (
	"regexp"
	"strings"
)

var gpLoginBusinessCode = regexp.MustCompile(`^获取 gp 域 JWT 失败: gp\.mcloud\.139\.com 登录失败: code=(-?[0-9]{1,10})(?:\s|$)`)

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
	status = TaskLogStatus(status)
	if status == "failed" && (taskType == "tasklist_gp" || taskType == "annual_bill" || taskType == "luckybag") {
		if match := gpLoginBusinessCode.FindStringSubmatch(strings.TrimSpace(raw)); match != nil {
			return "GP 渠道登录失败（上游业务码 " + match[1] + "），本次任务未执行"
		}
	}
	message := Message(taskType, raw)
	if status == "pending" {
		switch message {
		case "执行中", "正在执行", "等待执行":
			return "执行已结束，结果待确认"
		}
	}
	return message
}
