package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
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

// CompleteAICameraTask 完成 AI 相机任务。
//
// 真实链路（对齐抓包 paizhaowenai/aijiugongge）：先把一张真实图片上传到云盘
// 拿到 fileId，再用该 fileId 以 sendType=3 调用 aiRecognize，最后发起带
// command 036/036006 与同一无线图片附件的灵犀对话。旧实现用 base64+sendType=2
// 且强依赖识图响应里的 fileId（服务端恒为 null），导致任务永不完成。
func (api *CaiyunAPI) CompleteAICameraTask() error {
	userDomainID := strings.TrimSpace(api.client.GetUserDomainID())
	if userDomainID == "" {
		return fmt.Errorf("缺少 userDomainId")
	}

	fileID, fileName, err := api.uploadAICameraSample()
	if err != nil {
		return err
	}

	recognizeBody := map[string]interface{}{
		"channelId":     "101",
		"userId":        userDomainID,
		"recognizeType": "1",
		"fileId":        fileID,
		"sendType":      "3",
		"imageExt":      "jpg",
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
		Success bool        `json:"success"`
		Code    interface{} `json:"code"`
		Message string      `json:"message"`
	}
	if err := json.Unmarshal([]byte(recognizeText), &recognizeResult); err != nil {
		return fmt.Errorf("解析 AI 相机识图响应失败: %w", err)
	}
	if !recognizeResult.Success && fmt.Sprint(recognizeResult.Code) != "0000" {
		return fmt.Errorf("AI 相机识图失败: %s", normalizeMessageText("", recognizeResult.Message))
	}

	cst := time.FixedZone("CST", 8*3600)
	chatBody := map[string]interface{}{
		"userId":          userDomainID,
		"sessionId":       "",
		"applicationType": "chat",
		"applicationId":   "",
		"sourceChannel":   "101",
		"dialogueInput": map[string]interface{}{
			"dialogue":                        "这张图片里有什么内容？",
			"prompt":                          "",
			"inputTime":                       time.Now().In(cst).Format("2006-01-02T15:04:05.000-07:00"),
			"enableForceLlm":                  false,
			"enableForceNetworkSearch":        true,
			"enableModelThinking":             false,
			"enableAllNetworkSearch":          false,
			"enableKnowledgeAndNetworkSearch": false,
			"enableRegenerate":                false,
			"versionInfo":                     map[string]interface{}{"h5Version": "3.1.2"},
			"command":                         map[string]interface{}{"command": "036", "subCommand": "036006"},
			"extInfo":                         "{}",
			"sortInfo":                        map[string]interface{}{},
			"toolSetting":                     map[string]interface{}{"imageToolSetting": map[string]interface{}{"enableLlmDescribe": true}},
			"attachment": map[string]interface{}{
				"attachmentTypeList": []int{3},
				"fileList": []map[string]interface{}{
					{"fileId": fileID, "name": fileName},
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

// uploadAICameraSample 将内置样例图片作为一张真实图片上传到云盘，
// 返回 AI 相机识图与对话所需的 fileId 和文件名。
func (api *CaiyunAPI) uploadAICameraSample() (string, string, error) {
	// 火山方舟视觉模型会拒收 1x1 退化图片，这里生成一张真实的纯色渐变
	// JPEG，保证 aiRecognize 能正常读图。
	content, err := encodeSampleJPEG(480, 640)
	if err != nil {
		return "", "", err
	}

	fileName := fmt.Sprintf("wx_camera_%d.jpg", time.Now().UnixMilli())
	fileAPI := NewFileAPI(api.client)
	uploaded, err := fileAPI.UploadRandomFile(&UploadRandomFileRequest{
		ParentFileID: "/",
		Name:         fileName,
		Content:      content,
		ChannelSrc:   "10000023",
		Ext:          ".jpg",
	})
	if err != nil {
		return "", "", err
	}
	if uploaded == nil || uploaded.FileID == "" {
		return "", "", fmt.Errorf("上传 AI 相机样例图失败：未返回 fileId")
	}
	return uploaded.FileID, fileName, nil
}

// GenerateSampleJPEG 生成一张合法的渐变 JPEG，供 AI 相机识图与备份上传复用。
func GenerateSampleJPEG(height, width int) ([]byte, error) {
	return encodeSampleJPEG(height, width)
}

// encodeSampleJPEG 生成一张宽 height、高 width 的渐变 JPEG（合法可解码图片）。
func encodeSampleJPEG(height, width int) ([]byte, error) {
	if height <= 0 {
		height = 480
	}
	if width <= 0 {
		width = 640
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 255) / width),
				G: uint8((y * 255) / height),
				B: 128,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("生成 AI 相机样例图失败: %w", err)
	}
	return buf.Bytes(), nil
}

// lingxiChatDialogues 是与 AI 灵犀对话任务配套的普通提问文案。
var lingxiChatDialogues = []string{
	"作为中国移动的灵犀，你有什么可以帮到我的吗？",
	"移动云盘有哪些实用的AI功能？",
	"帮我推荐一个整理云盘照片的方法。",
}

// CompleteLingxiChat 与 AI 灵犀进行一次纯文本对话（无附件），
// 用于完成“与灵犀对话/和AI助手对话/AI_LINGXI”类任务。
func (api *CaiyunAPI) CompleteLingxiChat() error {
	userDomainID := strings.TrimSpace(api.client.GetUserDomainID())
	if userDomainID == "" {
		return fmt.Errorf("缺少 userDomainId")
	}

	cst := time.FixedZone("CST", 8*3600)
	dialogue := lingxiChatDialogues[int(time.Now().Unix())%len(lingxiChatDialogues)]
	chatBody := map[string]interface{}{
		"userId":          userDomainID,
		"sessionId":       "",
		"applicationType": "chat",
		"applicationId":   "",
		"sourceChannel":   "101",
		"dialogueInput": map[string]interface{}{
			"dialogue":                        dialogue,
			"prompt":                          "你是一位精通古今中外各个领域知识的专家，请实事求是、客观中立地回答我的问题。",
			"inputTime":                       time.Now().In(cst).Format("2006-01-02T15:04:05.000-07:00"),
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
	return fmt.Errorf("灵犀对话失败: 响应未包含有效回答")
}
