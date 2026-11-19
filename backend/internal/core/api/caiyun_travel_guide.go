package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GenerateTravelGuide performs the text conversation used by task 588. Success
// here only acknowledges the AI request; the task runner must re-read FINISH.
func (api *CaiyunAPI) GenerateTravelGuide() error {
	userID := strings.TrimSpace(api.client.GetUserDomainID())
	if userID == "" {
		return fmt.Errorf("出行攻略缺少 userDomainId")
	}
	body := map[string]interface{}{
		"userId":          userID,
		"sessionId":       "",
		"applicationType": "chat",
		"applicationId":   "",
		"sourceChannel":   "101",
		"dialogueInput": map[string]interface{}{
			"dialogue":                        "给我一份五一出行攻略",
			"prompt":                          "",
			"inputTime":                       time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02T15:04:05.000-07:00"),
			"enableForceLlm":                  false,
			"enableForceNetworkSearch":        true,
			"enableModelThinking":             false,
			"enableAllNetworkSearch":          false,
			"enableKnowledgeAndNetworkSearch": false,
			"enableRegenerate":                false,
			"versionInfo":                     map[string]interface{}{"h5Version": "3.1.2"},
			"extInfo":                         "{}",
			"sortInfo":                        map[string]interface{}{},
			"toolSetting":                     map[string]interface{}{"imageToolSetting": map[string]interface{}{"enableLlmDescribe": true}},
			"attachment":                      map[string]interface{}{},
		},
	}
	resp, err := api.client.PostWithTimeout(AIYunURL+"/api/outer/assistant/chat/v2/add", api.buildAIHeaders(true), body, 90*time.Second)
	if err != nil {
		return fmt.Errorf("出行攻略请求失败: %w", err)
	}
	text, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("读取出行攻略响应失败: %w", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("出行攻略 HTTP %d", resp.StatusCode)
	}
	if err := validateAIChatResponse(text); err != nil {
		return fmt.Errorf("出行攻略: %w", err)
	}
	return nil
}

// Inspect the entire response: an earlier successful SSE acknowledgement must
// not hide a later business error. Empty/HTML/malformed responses are failures.
func validateAIChatResponse(text string) error {
	acknowledged := false
	checkEvent := func(payload string) error {
		if payload == "[DONE]" {
			return nil
		}
		var event map[string]json.RawMessage
		if err := json.Unmarshal([]byte(payload), &event); err != nil || event == nil {
			return fmt.Errorf("AI 对话响应不是有效 JSON 事件")
		}
		if value, exists := event["error"]; exists && string(value) != "null" && string(value) != `""` {
			return fmt.Errorf("AI 对话返回错误事件")
		}
		if value, exists := event["success"]; exists {
			var success bool
			if err := json.Unmarshal(value, &success); err != nil || !success {
				return fmt.Errorf("AI 对话业务失败")
			}
			acknowledged = true
		}
		if value, exists := event["code"]; exists {
			var code string
			if err := json.Unmarshal(value, &code); err != nil || code != "0000" {
				return fmt.Errorf("AI 对话业务码异常: %s", code)
			}
			acknowledged = true
		}
		return nil
	}
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "{") {
		if err := checkEvent(text); err != nil {
			return err
		}
	} else {
		var data []string
		eventType := ""
		flush := func() error {
			if strings.EqualFold(eventType, "error") {
				return fmt.Errorf("AI 对话返回 SSE 错误事件")
			}
			if len(data) > 0 {
				if err := checkEvent(strings.TrimSpace(strings.Join(data, "\n"))); err != nil {
					return err
				}
			}
			data, eventType = nil, ""
			return nil
		}
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSuffix(line, "\r")
			if line == "" {
				if err := flush(); err != nil {
					return err
				}
			} else if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			} else if strings.HasPrefix(line, "event:") {
				eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			} else if !strings.HasPrefix(line, ":") && !strings.HasPrefix(line, "id:") && !strings.HasPrefix(line, "retry:") {
				return fmt.Errorf("AI 对话响应不是有效 SSE")
			}
		}
		if err := flush(); err != nil {
			return err
		}
	}
	if !acknowledged {
		return fmt.Errorf("AI 对话响应缺少有效成功事件")
	}
	return nil
}
