package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func (api *CaiyunAPI) buildAIHeaders(useClientInfo bool) map[string]string {
	tid := fmt.Sprintf("%d", time.Now().UnixNano())
	headers := map[string]string{
		"Connection":         "keep-alive",
		"sec-ch-ua-platform": "\"Android\"",
		"x-yun-api-version":  "v1",
		"x-yun-tid":          tid,
		"sec-ch-ua":          "\"Android WebView\";v=\"143\", \"Chromium\";v=\"143\", \"Not A(Brand\";v=\"24\"",
		"sec-ch-ua-mobile":   "?1",
		"X-Requested-With":   "com.chinamobile.mcloud",
		"Origin":             "https://frontend.mcloud.139.com",
		"Referer":            "https://frontend.mcloud.139.com/",
		"User-Agent":         "Mozilla/5.0 (Linux; Android 10; MI 8 Build/QKQ1.190828.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/143.0.7499.146 Mobile Safari/537.36 MCloudApp/12.5.4 tid/" + tid,
		"Content-Type":       "application/json",
		"Sec-Fetch-Site":     "same-site",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Dest":     "empty",
		"Accept-Encoding":    "gzip, deflate, br, zstd",
		"Accept-Language":    "zh,zh-CN;q=0.9,en-US;q=0.8,en;q=0.7",
	}
	if useClientInfo {
		headers["Accept"] = "text/event-stream"
		headers["x-yun-client-info"] = "4||1|12.5.4||MI 8|" + tid + "||android 10|||||"
		headers["x-yun-app-channel"] = "101"
		return headers
	}
	headers["Accept"] = "*/*"
	headers["x-DeviceInfo"] = "||36|12.5.4||MI 8|" + tid + "||android 10|||||"
	return headers
}

func (api *CaiyunAPI) isAICameraChatSuccess(text string) bool {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		payload := strings.TrimSpace(line)
		if strings.HasPrefix(payload, "data:") {
			payload = strings.TrimSpace(strings.TrimPrefix(payload, "data:"))
		}
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			continue
		}
		if boolFromAny(data["success"]) {
			return true
		}
		if code, ok := data["code"].(string); ok && code == "0000" {
			return true
		}
	}
	return false
}

// CompleteAICameraTask 完成 AI 相机任务
func (api *CaiyunAPI) CompleteAICameraTask() error {
	userDomainID := strings.TrimSpace(api.client.GetUserDomainID())
	if userDomainID == "" {
		return fmt.Errorf("缺少 userDomainId")
	}

	recognizeBody := map[string]interface{}{
		"channelId":     "101",
		"userId":        userDomainID,
		"recognizeType": "1",
		"base64":        AICameraSampleBase64,
		"sendType":      "2",
		"imageExt":      "jpg",
		"uploadToCloud": true,
		"timeout":       30000,
	}
	recognizeResp, err := api.client.Post(AIYunURL+"/api/image/aiRecognize", api.buildAIHeaders(false), recognizeBody)
	if err != nil {
		return err
	}

	recognizeText, err := api.client.ReadResponseBody(recognizeResp)
	if err != nil {
		return err
	}

	var recognizeResult struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			FileID string      `json:"fileId"`
			TaskID interface{} `json:"taskId"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(recognizeText), &recognizeResult); err != nil {
		return fmt.Errorf("解析 AI 相机识图响应失败: %w", err)
	}
	if !recognizeResult.Success {
		return fmt.Errorf("AI 相机识图失败: %s", normalizeMessageText("", recognizeResult.Message))
	}
	if recognizeResult.Data.FileID == "" {
		return fmt.Errorf("AI 相机识图失败: 缺少 fileId")
	}

	cst := time.FixedZone("CST", 8*3600)
	fileName := fmt.Sprintf("%d.jpeg", time.Now().UnixMilli())
	chatBody := map[string]interface{}{
		"userId":          userDomainID,
		"sessionId":       "",
		"applicationType": "chat",
		"applicationId":   "",
		"sourceChannel":   "101",
		"dialogueInput": map[string]interface{}{
			"dialogue":                        "？",
			"prompt":                          "",
			"inputTime":                       time.Now().In(cst).Format("2006-01-02T15:04:05.000-07:00"),
			"enableForceLlm":                  false,
			"enableForceNetworkSearch":        true,
			"enableModelThinking":             false,
			"enableAllNetworkSearch":          false,
			"enableKnowledgeAndNetworkSearch": false,
			"enableRegenerate":                false,
			"versionInfo":                     map[string]interface{}{"h5Version": "2.7.6"},
			"extInfo":                         "{}",
			"sortInfo":                        map[string]interface{}{},
			"toolSetting":                     map[string]interface{}{"imageToolSetting": map[string]interface{}{"enableLlmDescribe": true}},
			"attachment": map[string]interface{}{
				"attachmentTypeList": []int{3},
				"fileList": []map[string]interface{}{
					{"fileId": recognizeResult.Data.FileID, "name": fileName},
				},
			},
		},
	}

	chatResp, err := api.client.Post(AIYunURL+"/api/outer/assistant/chat/v2/add", api.buildAIHeaders(true), chatBody)
	if err != nil {
		return err
	}
	chatText, err := api.client.ReadResponseBody(chatResp)
	if err != nil {
		return err
	}
	if api.isAICameraChatSuccess(chatText) {
		return nil
	}

	var chatResult map[string]interface{}
	if err := json.Unmarshal([]byte(chatText), &chatResult); err == nil {
		if boolFromAny(chatResult["success"]) {
			return nil
		}
		if code, ok := chatResult["code"].(string); ok && code == "0000" {
			return nil
		}
		msg := normalizeMessageText(fmt.Sprint(chatResult["msg"]), fmt.Sprint(chatResult["message"]))
		if msg == "" {
			msg = "响应解析失败"
		}
		return fmt.Errorf("AI 相机对话失败: %s", msg)
	}

	return fmt.Errorf("AI 相机对话失败: 响应解析失败")
}
