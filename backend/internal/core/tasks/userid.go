package tasks

import (
	"fmt"
	"regexp"
	"strings"

	"caiyun/internal/core/api"
)

var noteAuthUserIDPattern = regexp.MustCompile(`"userId"\s*:\s*"?(\d+)"?`)

// ResolveUserID 通过云笔记鉴权响应解析 AI 任务依赖的 userID。
func ResolveUserID(apiClient *api.CaiyunAPI, authToken, phone string) (string, error) {
	if apiClient == nil {
		return "", fmt.Errorf("CaiyunAPI 未初始化")
	}
	if strings.TrimSpace(authToken) == "" || strings.TrimSpace(phone) == "" {
		return "", fmt.Errorf("解析 userID 缺少 authToken 或手机号")
	}

	noteAuth, err := apiClient.GetNoteAuthToken(strings.TrimSpace(authToken), strings.TrimSpace(phone))
	if err != nil {
		return "", err
	}
	if noteAuth == nil || strings.TrimSpace(noteAuth.Body) == "" {
		return "", fmt.Errorf("云笔记鉴权响应为空")
	}

	match := noteAuthUserIDPattern.FindStringSubmatch(noteAuth.Body)
	if len(match) != 2 {
		return "", fmt.Errorf("云笔记鉴权响应中未找到 userID")
	}
	return match[1], nil
}
