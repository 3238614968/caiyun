package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// CompleteFunAIKnowledgeUse uses a task-owned library, never imports into an
// arbitrary user library. fileID is returned even on failure for batch cleanup.
func (api *CaiyunAPI) CompleteFunAIKnowledgeUse() (fileID string, err error) {
	if err = api.CreateTaskKnowledgeBase(); err != nil {
		return
	}
	uid := api.client.GetUserDomainID()
	call := func(path string, body interface{}) (json.RawMessage, error) {
		resp, e := api.client.PostWithTimeout(AIYunURL+"/api/outer"+path, api.buildAIHeaders(false), body, 60*time.Second)
		if e != nil {
			return nil, e
		}
		text, e := api.client.ReadResponseBody(resp)
		if e != nil {
			return nil, e
		}
		var r struct {
			Code    interface{}     `json:"code"`
			Data    json.RawMessage `json:"data"`
			Message string          `json:"message"`
		}
		if e = json.Unmarshal([]byte(text), &r); e != nil {
			return nil, e
		}
		if resp.StatusCode != 200 || fmt.Sprint(r.Code) != "0000" {
			return nil, fmt.Errorf("知识库接口 %s: HTTP %d code=%v %s", path, resp.StatusCode, r.Code, r.Message)
		}
		return r.Data, nil
	}
	data, err := call("/assistant/knowledge/personal/v2/base/list", map[string]string{"sourceChannel": "1040000", "userId": uid})
	if err != nil {
		return fileID, err
	}
	var list struct {
		Bases []struct {
			ID   string `json:"baseId"`
			Name string `json:"name"`
		} `json:"baseList"`
	}
	if err = json.Unmarshal(data, &list); err != nil {
		return fileID, err
	}
	id, name := "", ""
	for _, base := range list.Bases {
		if base.Name == "云朵中心任务知识库" || strings.HasPrefix(base.Name, "云盘任务知识库-") {
			id, name = base.ID, base.Name
			break
		}
	}
	if id == "" {
		return fileID, fmt.Errorf("未查询到任务知识库")
	}
	uploaded, err := NewFileAPI(api.client).UploadRandomFile(&UploadRandomFileRequest{Name: fmt.Sprintf("auto_kb_%d.txt", time.Now().UnixNano()), Content: []byte("移动云盘知识库任务文档。知识库支持整理和总结云盘文件。"), ChannelSrc: "10000023"})
	if err != nil {
		return fileID, err
	}
	if uploaded == nil || uploaded.FileID == "" {
		return fileID, fmt.Errorf("知识库素材上传缺少 fileId")
	}
	fileID = uploaded.FileID
	_, err = call("/assistant/knowledge/personal/v2/base/file/batchImport", map[string]interface{}{"sourceChannel": "1040000", "userId": uid, "baseId": id, "parentFileId": "", "resourceType": 0, "fileList": []map[string]string{{"fileId": fileID, "type": "file"}}})
	if err != nil {
		return
	}
	_, err = call("/assistant/chat/config", map[string]interface{}{"sourceChannel": "1040000", "userId": uid, "baseIdList": []string{id}})
	if err != nil {
		return
	}
	resp, err := api.client.PostWithTimeout(AIYunURL+"/api/outer/assistant/chat/v2/add", api.buildAIHeaders(true), map[string]interface{}{
		"userId": uid, "sessionId": "", "applicationType": "chat", "applicationId": "", "sourceChannel": "1040023",
		"dialogueInput": map[string]interface{}{"dialogue": "总结一下这个知识库的内容", "prompt": "", "inputTime": time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02T15:04:05.000-07:00"), "versionInfo": map[string]string{"h5Version": "3.4.0"}, "attachment": map[string]interface{}{"attachmentTypeList": []int{8}, "fileList": []map[string]interface{}{{"id": id, "name": name, "type": 8, "category": "knowledgeBase"}}}},
	}, 90*time.Second)
	if err != nil {
		return
	}
	text, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return
	}
	if resp.StatusCode != 200 || !api.isAICameraChatSuccess(text) {
		return fileID, fmt.Errorf("知识库对话未成功")
	}
	return fileID, nil
}
