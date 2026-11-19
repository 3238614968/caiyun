package api

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// uploadKnowledgeMaterial uses the official knowledge uploader, which creates
// the resource inside the selected personal library. It is distinct from a
// normal cloud upload followed by the asynchronous batch-import queue.
func (api *CaiyunAPI) uploadKnowledgeMaterial(baseID, name string, content []byte) (string, error) {
	if baseID == "" || name == "" || len(content) == 0 {
		return "", fmt.Errorf("知识库上传参数不完整")
	}
	const root = AIYunURL + "/api/outer/assistant/knowledge/personal/v2/file/upload/"
	call := func(path string, body interface{}) (json.RawMessage, error) {
		resp, err := api.client.Post(root+path, api.buildAIHeaders(false), body)
		if err != nil {
			return nil, err
		}
		text, err := api.client.ReadResponseBody(resp)
		if err != nil {
			return nil, err
		}
		var envelope struct {
			Data json.RawMessage `json:"data"`
			Code interface{}     `json:"code"`
		}
		if err := json.Unmarshal([]byte(text), &envelope); err != nil {
			return nil, err
		}
		if err := validateFileUploadResponse(resp, text); err != nil {
			return nil, fmt.Errorf("知识库上传 %s code=%v: %w", path, envelope.Code, err)
		}
		return envelope.Data, nil
	}
	hash := sha256Hex(content)
	data, err := call("createFile", map[string]interface{}{
		"sourceChannel": "1040000", "baseId": baseID, "parentFileId": "", "fileRenameMode": "auto_rename",
		"name": name, "size": len(content), "contentHashAlgorithm": "sha256", "contentHash": hash, "parallelUpload": false,
		"partInfos": []map[string]interface{}{{"parallelHashCtx": map[string]int{"partOffset": 0}, "partNumber": 1, "partSize": len(content)}},
	})
	if err != nil {
		return "", err
	}
	var created struct {
		FileID   string `json:"fileId"`
		UploadID string `json:"uploadId"`
		Rapid    bool   `json:"rapidUpload"`
		Exist    bool   `json:"exist"`
		Parts    []struct {
			URL string `json:"uploadUrl"`
		} `json:"partInfos"`
	}
	if json.Unmarshal(data, &created) != nil || created.FileID == "" {
		return "", fmt.Errorf("知识库上传创建未返回 fileId")
	}
	if created.Rapid || created.Exist {
		return created.FileID, nil
	}
	if created.UploadID == "" || len(created.Parts) != 1 || strings.TrimSpace(created.Parts[0].URL) == "" {
		return created.FileID, fmt.Errorf("知识库上传缺少单片上传地址或 uploadId")
	}
	resp, err := api.client.PutPresigned(created.Parts[0].URL, "application/octet-stream", content)
	if err != nil {
		return created.FileID, err
	}
	if resp == nil || resp.Body == nil {
		return created.FileID, fmt.Errorf("知识库上传 PUT 响应为空")
	}
	_, readErr := io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return created.FileID, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return created.FileID, fmt.Errorf("知识库上传 PUT HTTP %d", resp.StatusCode)
	}
	_, err = call("complete", map[string]interface{}{"sourceChannel": "1040000", "baseId": baseID, "fileId": created.FileID, "uploadId": created.UploadID, "contentHash": hash, "contentHashAlgorithm": "sha256"})
	return created.FileID, err
}
