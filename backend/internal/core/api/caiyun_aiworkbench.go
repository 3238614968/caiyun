package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const mailAIWorkbenchChannel = "10175"

func aiWorkbenchDialogueID(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data:"))
		var event map[string]interface{}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if id := miaoyunString(event["dialogueId"]); id != "" {
			return id
		}
		if data, ok := event["data"].(map[string]interface{}); ok {
			if id := miaoyunString(data["dialogueId"]); id != "" {
				return id
			}
		}
	}
	return ""
}

// CompleteAIWorkbenchTask uses the 139 Mail workbench's channel, which differs
// from the cloud app's Lingxi channel. The chat action is the task condition.
func (api *CaiyunAPI) CompleteAIWorkbenchTask() error {
	userID := strings.TrimSpace(api.client.GetUserDomainID())
	if userID == "" {
		return fmt.Errorf("AI 工作台缺少 userDomainId")
	}
	inputTime := time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02T15:04:05.000-07:00")
	var dialogueID string
	chatSucceeded := false
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}
		resp, err := api.client.Post(AIYunURL+"/api/outer/assistant/chat/v2/add", api.buildAIHeaders(true), map[string]interface{}{
			"userId": userID, "sessionId": "", "applicationType": "chat", "applicationId": "",
			"sourceChannel": mailAIWorkbenchChannel,
			"dialogueInput": map[string]interface{}{
				"dialogue": "帮我写一篇云盘使用心得", "prompt": "", "inputTime": inputTime,
				"versionInfo": map[string]string{"h5Version": "3.4.0"},
			},
		})
		if err != nil {
			continue
		}
		body, err := api.client.ReadResponseBody(resp)
		if err != nil {
			continue
		}
		if api.isAICameraChatSuccess(body) {
			chatSucceeded = true
			if dialogueID == "" {
				dialogueID = aiWorkbenchDialogueID(body)
			}
		}
	}
	if !chatSucceeded {
		return fmt.Errorf("139 邮箱 AI 工作台对话未成功")
	}
	// Writing endpoints are part of the workbench experience but do not control
	// its task completion; a failed optional call must not hide a completed chat.
	requests := []struct {
		path string
		body map[string]interface{}
	}{
		{"/assistant/aiwriting/keyword/generate", map[string]interface{}{"userId": userID, "title": "云盘使用心得"}},
		{"/assistant/chat/config/get", map[string]interface{}{"userId": userID}},
	}
	if dialogueID != "" {
		requests = append(requests, struct {
			path string
			body map[string]interface{}
		}{"/assistant/aiwriting/chat/update", map[string]interface{}{
			"dialogueId": dialogueID, "sourceChannel": mailAIWorkbenchChannel,
			"userId": userID, "connentType": "2", "htmlContent": "<p>云盘使用心得：文件同步很方便。</p>",
		}})
	}
	for _, request := range requests {
		resp, err := api.client.Post(AIYunURL+"/api/outer"+request.path, api.buildAIHeaders(false), request.body)
		if err == nil {
			_, _ = api.client.ReadResponseBody(resp)
		}
	}
	return nil
}
