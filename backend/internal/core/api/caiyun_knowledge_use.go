package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// KnowledgeIndexPendingError keeps the cloud source alive while the official
// indexer is working. A successful import response alone is not a ready index.
type KnowledgeIndexPendingError struct{ Cause error }

func (e *KnowledgeIndexPendingError) Error() string {
	if e.Cause != nil {
		return "知识库解析状态未确认: " + e.Cause.Error()
	}
	return "知识库素材仍在解析，等待服务端入库后重试"
}

func (e *KnowledgeIndexPendingError) Unwrap() error { return e.Cause }

type knowledgeResource struct {
	ID       string `json:"resourceId"`
	Name     string `json:"name"`
	AIStatus *int   `json:"aiStatus"`
}

// CompleteFunAIKnowledgeUse uses a task-owned library, never imports into an
// arbitrary user library. A pending index must retain its source file.
func (api *CaiyunAPI) CompleteFunAIKnowledgeUse() (fileID string, err error) {
	return api.completeFunAIKnowledgeUse(time.Sleep)
}

func (api *CaiyunAPI) completeFunAIKnowledgeUse(wait func(time.Duration)) (fileID string, err error) {
	// kbMaterialName 知识库素材文件名（上传与 chat 附件要保持一致）。
	kbMaterialName := fmt.Sprintf("auto_kb_%d_%s.txt", time.Now().UnixNano(), uuid.NewString())
	now := time.Now()
	libraryName := taskKnowledgeBaseName(now, "1040000")
	if err = api.createKnowledgeBaseForSource(now, "1040000"); err != nil {
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
			Success *bool           `json:"success"`
			Data    json.RawMessage `json:"data"`
			Message string          `json:"message"`
		}
		if e = json.Unmarshal([]byte(text), &r); e != nil {
			return nil, e
		}
		if resp.StatusCode != 200 || fmt.Sprint(r.Code) != "0000" || r.Success != nil && !*r.Success {
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
			ID       string `json:"baseId"`
			Name     string `json:"name"`
			BaseType int    `json:"baseType"`
		} `json:"baseList"`
	}
	if err = json.Unmarshal(data, &list); err != nil {
		return fileID, err
	}
	id, name, baseType := "", "", 0
	for _, base := range list.Bases {
		if base.Name == libraryName {
			id = base.ID
			name, baseType = base.Name, base.BaseType
			break
		}
	}
	if id == "" {
		return fileID, fmt.Errorf("未查询到任务知识库")
	}
	resources := func() ([]knowledgeResource, error) {
		data, e := call("/assistant/knowledge/personal/v2/base/resource/list", map[string]interface{}{
			"sourceChannel": "1040000", "userId": uid, "baseId": id,
			"parentResoureId": nil, "sortDirection": 2, "sortType": 1,
			"aiStatus": nil, "resourceType": nil,
			"pageInfo": map[string]interface{}{"pageSize": 100, "pageCursor": nil, "needTotalCount": 1},
		})
		if e != nil {
			return nil, e
		}
		var page struct {
			List json.RawMessage `json:"list"`
		}
		if e := json.Unmarshal(data, &page); e != nil || len(page.List) == 0 {
			return nil, fmt.Errorf("知识库资源列表无效")
		}
		// The official empty personal-library response uses list:null.
		var items []knowledgeResource
		if json.Unmarshal(page.List, &items) != nil {
			return nil, fmt.Errorf("知识库资源列表无效")
		}
		return items, nil
	}
	items, err := resources()
	if err != nil {
		return fileID, err
	}
	ready, pending := false, false
	for _, item := range items {
		if item.ID == "" || item.AIStatus == nil {
			continue
		}
		ready = ready || *item.AIStatus == 1
		pending = pending || *item.AIStatus == 0
	}
	if !ready && !pending {
		// A reused byte-for-byte document can hit rapidUpload and replay an old
		// import. Match the reference's unique document while retaining a valid file.
		material := strings.Repeat(fmt.Sprintf("移动云盘知识库任务文档 %s。知识库支持整理和总结云盘文件。", kbMaterialName), 8)
		fileID, err = api.uploadKnowledgeMaterial(id, kbMaterialName, []byte(material))
		if err != nil {
			return fileID, err
		}
	}
	// Match the official UI: only parsed resources are offered to a knowledge
	// conversation. Reuse the owned library instead of uploading every cron run.
	for attempt := 0; !ready && attempt < 6; attempt++ {
		if attempt > 0 {
			wait(5 * time.Second)
		}
		items, err = resources()
		if err != nil {
			return fileID, &KnowledgeIndexPendingError{Cause: err}
		}
		for _, item := range items {
			if item.ID != "" && item.AIStatus != nil && *item.AIStatus == 1 {
				ready = true
				break
			}
		}
	}
	if !ready {
		return fileID, &KnowledgeIndexPendingError{}
	}
	_, err = call("/assistant/chat/config", map[string]interface{}{"sourceChannel": "1040000", "userId": uid, "baseIdList": []string{id}})
	if err != nil {
		return
	}
	// FILEINFO_TYPE.knowledgeBase is 8, separate from the cloud-file attachment
	// type 10. The official builder sends knowledgeBaseList, not fileList.
	resp, err := api.client.PostWithTimeout(AIYunURL+"/api/outer/assistant/chat/v2/add", api.buildAIHeaders(true), map[string]interface{}{
		"userId": uid, "sessionId": "", "applicationType": "chat", "applicationId": "", "sourceChannel": "1040000",
		"dialogueInput": map[string]interface{}{"dialogue": "总结一下这个知识库的内容", "prompt": "", "inputTime": time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02T15:04:05.000-07:00"), "versionInfo": map[string]string{"h5Version": "2.3.2"}, "extInfo": `{"showBasedKnowledge":true}`, "attachment": map[string]interface{}{"attachmentTypeList": []int{8}, "knowledgeBaseList": []map[string]interface{}{{"baseId": id, "name": name, "baseType": baseType}}}},
	}, 90*time.Second)
	if err != nil {
		return
	}
	text, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return
	}
	if resp.StatusCode != 200 {
		return fileID, fmt.Errorf("知识库对话 HTTP %d", resp.StatusCode)
	}
	if err := validateAIChatResponse(text); err != nil {
		return fileID, fmt.Errorf("知识库对话未成功: %w", err)
	}
	return fileID, nil
}
