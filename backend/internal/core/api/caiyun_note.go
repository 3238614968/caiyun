package api

import (
	"encoding/json"
	"fmt"
	"time"
)

type NoteAuthResult struct {
	Body    string
	Headers map[string]string
}

// GetNoteAuthToken 获取云笔记鉴权头
func (api *CaiyunAPI) GetNoteAuthToken(authToken, phone string) (*NoteAuthResult, error) {
	if authToken == "" || phone == "" {
		return nil, fmt.Errorf("云笔记鉴权参数不完整")
	}

	headers := map[string]string{
		"APP_CP":              "android",
		"APP_NUMBER":          phone,
		"CP_VERSION":          "3.2.0",
		"x-huawei-channelsrc": "10001400",
		"User-Agent":          "mobile",
		"Content-Type":        "application/json",
	}
	bodyReq := map[string]interface{}{
		"authToken": authToken,
		"userPhone": phone,
	}

	resp, err := api.client.Post("https://note.mcloud.139.com/noteServer/api/authTokenRefresh.do", headers, bodyReq)
	if err != nil {
		return nil, err
	}

	appAuth := resp.Header.Get("app_auth")
	if appAuth == "" {
		appAuth = resp.Header.Get("App_Auth")
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	resultHeaders := map[string]string{}
	if appAuth != "" {
		resultHeaders["app_auth"] = appAuth
	}

	return &NoteAuthResult{
		Body:    body,
		Headers: resultHeaders,
	}, nil
}

// CreateNote 创建云笔记
func (api *CaiyunAPI) CreateNote(noteID, title, phone string, extraHeaders map[string]string, tags []string) error {
	if noteID == "" || phone == "" {
		return fmt.Errorf("创建云笔记参数不完整")
	}

	nowMs := fmt.Sprintf("%d", time.Now().UnixMilli())
	headers := map[string]string{
		"APP_CP":       "pc",
		"APP_NUMBER":   phone,
		"CP_VERSION":   "7.7.1.20240115",
		"Content-Type": "application/json",
	}
	for k, v := range extraHeaders {
		if v != "" {
			headers[k] = v
		}
	}

	reqBody := map[string]interface{}{
		"archived":        0,
		"attachmentdir":   "",
		"attachmentdirid": "",
		"attachments":     []interface{}{},
		"contentid":       "",
		"contents": []map[string]interface{}{
			{"data": "<span></span>", "noteId": noteID, "sortOrder": 0, "type": "TEXT"},
		},
		"cp":          "",
		"createtime":  nowMs,
		"description": "",
		"expands":     map[string]interface{}{"noteType": 0},
		"landMark":    []interface{}{},
		"latlng":      "",
		"location":    "",
		"noteid":      noteID,
		"remindtime":  "",
		"remindtype":  0,
		"revision":    "1",
		"system":      "",
		"tags":        tags,
		"title":       title,
		"topmost":     "0",
		"updatetime":  nowMs,
		"userphone":   phone,
		"version":     "",
		"visitTime":   nowMs,
	}

	resp, err := api.client.Post("https://mnote.caiyun.feixin.10086.cn/noteServer/api/createNote.do", headers, reqBody)
	if err != nil {
		return err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err == nil {
		if code, ok := result["code"]; ok && fmt.Sprint(code) != "0" && fmt.Sprint(code) != "" {
			msg := fmt.Sprint(result["msg"])
			if msg == "" {
				msg = fmt.Sprint(result["message"])
			}
			return fmt.Errorf("创建云笔记失败: code=%v, msg=%s", code, msg)
		}
	}
	return nil
}

// DeleteNote 删除云笔记
func (api *CaiyunAPI) DeleteNote(noteID string, extraHeaders map[string]string) error {
	if noteID == "" {
		return fmt.Errorf("noteID 为空")
	}

	headers := map[string]string{
		"APP_CP":       "pc",
		"CP_VERSION":   "7.7.1.20240115",
		"Content-Type": "application/json",
	}
	for k, v := range extraHeaders {
		if v != "" {
			headers[k] = v
		}
	}

	reqBody := map[string]interface{}{
		"noteids": []map[string]string{{"noteid": noteID}},
	}

	resp, err := api.client.Post("https://mnote.caiyun.feixin.10086.cn/noteServer/api/moveToRecycleBin.do", headers, reqBody)
	if err != nil {
		return err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err == nil {
		if code, ok := result["code"]; ok {
			codeStr := fmt.Sprint(code)
			if codeStr != "" && codeStr != "0" {
				msg := fmt.Sprint(result["msg"])
				if msg == "" {
					msg = fmt.Sprint(result["message"])
				}
				return fmt.Errorf("删除云笔记失败: code=%v, msg=%s", code, msg)
			}
		}
	}
	return nil
}

// OutLinkResponse 分享文件响应
