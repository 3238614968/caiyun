package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CreateTaskKnowledgeBase performs the real knowledge-base creation required by
// sign-in center task 547. The server task state remains the source of truth.
func (api *CaiyunAPI) CreateTaskKnowledgeBase() error {
	userID := strings.TrimSpace(api.client.GetUserDomainID())
	if userID == "" {
		return fmt.Errorf("缺少 userDomainId")
	}
	const name = "云朵中心任务知识库"
	listResp, err := api.client.Post(AIYunURL+"/api/outer/assistant/knowledge/personal/v2/base/list", api.buildAIHeaders(false), map[string]string{"sourceChannel": "10104", "userId": userID})
	if err != nil {
		return err
	}
	listBody, err := api.client.ReadResponseBody(listResp)
	if err != nil {
		return err
	}
	var existing struct {
		Code interface{} `json:"code"`
		Data struct {
			BaseList []struct {
				Name string `json:"name"`
			} `json:"baseList"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(listBody), &existing); err != nil {
		return err
	}
	if !isSuccessCode(existing.Code) && fmt.Sprint(existing.Code) != "0000" {
		return fmt.Errorf("知识库查重失败: code=%v", existing.Code)
	}
	for _, base := range existing.Data.BaseList {
		if base.Name == name || strings.HasPrefix(base.Name, "云盘任务知识库-") {
			return nil
		}
	}
	resp, err := api.client.Post(
		AIYunURL+"/api/outer/assistant/knowledge/personal/v2/base/create",
		api.buildAIHeaders(false),
		map[string]interface{}{
			"sourceChannel":     "10104",
			"userId":            userID,
			"knowledgeBaseName": name,
			"name":              name,
			"description":       "云朵中心任务知识库",
			"visibility":        "0",
		},
	)
	if err != nil {
		return err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	var result struct {
		Code    interface{} `json:"code"`
		Success bool        `json:"success"`
		Message string      `json:"message"`
		Msg     string      `json:"msg"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return fmt.Errorf("解析建知识库响应失败: %w", err)
	}
	if result.Success || isSuccessCode(result.Code) || fmt.Sprint(result.Code) == "0000" {
		return nil
	}
	return fmt.Errorf("建知识库失败: code=%v msg=%s", result.Code, normalizeMessageText(result.Msg, result.Message))
}
