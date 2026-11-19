package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// CreateTaskKnowledgeBase performs the real knowledge-base creation required by
// sign-in center task 547. The server task state remains the source of truth.
func (api *CaiyunAPI) CreateTaskKnowledgeBase() error {
	return api.createTaskKnowledgeBase(time.Now())
}

// CreateFunAIKnowledgeBase follows the verified App creation flow, including
// its official create-click event. Only the activity's later readback confirms
// task completion; neither the event receipt nor the baseId is a completion bit.
// A different set of participating modules gets a new real action rather than
// replaying a creation event for a library made before that entry was registered.
func (api *CaiyunAPI) CreateFunAIKnowledgeBase(moduleIDs ...string) error {
	now := time.Now()
	name := taskKnowledgeBaseName(now, "101") + "-App"
	if len(moduleIDs) > 0 {
		unique := make(map[string]bool)
		for _, id := range moduleIDs {
			if id = strings.TrimSpace(id); id != "" {
				unique[id] = true
			}
		}
		if len(unique) == 0 {
			return fmt.Errorf("知识库创建缺少有效活动模块")
		}
		ids := make([]string, 0, len(unique))
		for id := range unique {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
		name += fmt.Sprintf("%x", sum)[:5]
	}
	return api.createKnowledgeBaseWithEntry("101", name, func() error {
		return api.reportKnowledgeCreateEvent(now)
	})
}

func (api *CaiyunAPI) createTaskKnowledgeBase(now time.Time) error {
	return api.createKnowledgeBaseForSource(now, "10104")
}

func taskKnowledgeBaseName(now time.Time, source string) string {
	name := "云盘任务知识库-" + now.In(time.FixedZone("CST", 8*3600)).Format("20060102")
	if source == "1040000" {
		name += "-AI"
	}
	return name
}

func (api *CaiyunAPI) createKnowledgeBaseForSource(now time.Time, source string) error {
	return api.createNamedKnowledgeBase(source, taskKnowledgeBaseName(now, source))
}

func (api *CaiyunAPI) createNamedKnowledgeBase(source, name string) error {
	return api.createKnowledgeBaseWithEntry(source, name, nil)
}

func (api *CaiyunAPI) createKnowledgeBaseWithEntry(source, name string, beforeCreate func() error) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 25 {
		return fmt.Errorf("知识库名称必须为1至25个字符")
	}
	userID := strings.TrimSpace(api.client.GetUserDomainID())
	if userID == "" {
		return fmt.Errorf("缺少 userDomainId")
	}
	// The activity requires creating a personal library. A library from an
	// earlier day is not today's creation; retries on the same day are idempotent.
	listResp, err := api.client.Post(AIYunURL+"/api/outer/assistant/knowledge/personal/v2/base/list", api.knowledgeHeaders(source), map[string]string{"sourceChannel": source, "userId": userID})
	if err != nil {
		return err
	}
	listBody, err := api.client.ReadResponseBody(listResp)
	if err != nil {
		return err
	}
	var existing struct {
		Code    interface{} `json:"code"`
		Success *bool       `json:"success"`
		Data    struct {
			BaseList []struct {
				ID        string `json:"baseId"`
				Name      string `json:"name"`
				BaseType  int    `json:"baseType"`
				OpenLevel *int   `json:"openLevel"`
				Creator   string `json:"createBy"`
			} `json:"baseList"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(listBody), &existing); err != nil {
		return err
	}
	if listResp.StatusCode != 200 || fmt.Sprint(existing.Code) != "0000" || existing.Success != nil && !*existing.Success {
		return fmt.Errorf("知识库查重失败: code=%v", existing.Code)
	}
	for _, base := range existing.Data.BaseList {
		if base.Name == name {
			if base.ID == "" || base.BaseType != 1 || base.OpenLevel == nil || *base.OpenLevel != 0 {
				return fmt.Errorf("当天任务知识库不是已确认的私人个人库")
			}
			if source == "101" && base.Creator != userID {
				return fmt.Errorf("当天任务知识库不是当前账号创建的私人库")
			}
			return nil
		}
	}
	payload := map[string]interface{}{
		"sourceChannel": source, "userId": userID, "name": name, "description": "云朵中心任务知识库", "openLevel": 0,
	}
	if source == "101" {
		photo, err := api.knowledgeDefaultAvatar(source, userID)
		if err != nil {
			return err
		}
		payload["profilePhoto"] = photo
	}
	if beforeCreate != nil {
		if err := beforeCreate(); err != nil {
			return err
		}
	}
	resp, err := api.client.Post(
		AIYunURL+"/api/outer/assistant/knowledge/personal/v2/base/create",
		api.knowledgeHeaders(source), payload,
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
		Success *bool       `json:"success"`
		Message string      `json:"message"`
		Msg     string      `json:"msg"`
		Data    struct {
			Base struct {
				ID string `json:"baseId"`
			} `json:"base"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return fmt.Errorf("解析建知识库响应失败: %w", err)
	}
	if resp.StatusCode == 200 && fmt.Sprint(result.Code) == "0000" && (result.Success == nil || *result.Success) {
		if result.Data.Base.ID == "" {
			return fmt.Errorf("建知识库回执缺少 baseId，未确认创建")
		}
		return nil
	}
	return fmt.Errorf("建知识库失败: code=%v msg=%s", result.Code, normalizeMessageText(result.Msg, result.Message))
}

func (api *CaiyunAPI) knowledgeHeaders(source string) map[string]string {
	headers := api.buildAIHeaders(false)
	if source != "101" {
		return headers
	}
	// Match the App SDK's getHeaderAiChat/xO descriptor. Keep this client's
	// configured device identity while using the current App terminal/version.
	parts := strings.Split(api.client.GetClientInfo(), "|")
	field := func(index int) string {
		if index < len(parts) {
			return parts[index]
		}
		return ""
	}
	info := fmt.Sprintf("4||1|%s|%s|%s|%s||%s|||||", MarketClientVersion, field(4), field(5), field(6), strings.ToLower(field(8)))
	headers["x-yun-client-info"], headers["x-DeviceInfo"] = info, info
	headers["x-yun-tid"] = uuid.NewString()
	headers["x-huawei-channelSrc"] = "10252400"
	headers["x-yun-uni"] = api.client.GetUserDomainID()
	headers["User-Agent"] = MarketUserAgent
	headers["Origin"] = "https://yun.139.com"
	headers["Referer"] = "https://yun.139.com/ai-helper/?sourceid=810292"
	for _, key := range []string{"sec-ch-ua", "sec-ch-ua-platform", "sec-ch-ua-mobile", "Sec-Fetch-Site", "Sec-Fetch-Mode", "Sec-Fetch-Dest"} {
		delete(headers, key)
	}
	return headers
}

func (api *CaiyunAPI) knowledgeDefaultAvatar(source, userID string) (map[string]interface{}, error) {
	resp, err := api.client.Post(AIYunURL+"/api/outer/assistant/knowledge/personal/v2/base/profilePhoto/list", api.knowledgeHeaders(source), map[string]string{"sourceChannel": source, "userId": userID})
	if err != nil {
		return nil, err
	}
	text, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if err := validateFileUploadResponse(resp, text); err != nil {
		return nil, fmt.Errorf("知识库默认头像查询: %w", err)
	}
	var envelope struct {
		Data []map[string]interface{} `json:"data"`
	}
	if json.Unmarshal([]byte(text), &envelope) != nil || len(envelope.Data) == 0 || envelope.Data[0]["photoId"] == nil {
		return nil, fmt.Errorf("知识库未返回有效的默认头像")
	}
	return envelope.Data[0], nil
}
