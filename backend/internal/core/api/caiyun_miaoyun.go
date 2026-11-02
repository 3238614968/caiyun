package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const miaoyunBaseURL = AIYunURL + "/aitools/miaoyun"

func (api *CaiyunAPI) miaoyunCall(endpoint string, payload interface{}) (map[string]interface{}, error) {
	timeout := 60 * time.Second
	if endpoint == "/image/createImage" || endpoint == "/art/autoComposeArtPhoto" {
		timeout = 90 * time.Second
	}
	resp, err := api.client.PostWithBasicAuth(miaoyunBaseURL+endpoint, payload,
		"application/json;charset:utf-8;", timeout)
	if err != nil {
		return nil, err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("妙云%s HTTP %d", endpoint, resp.StatusCode)
	}
	var result map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("解析妙云%s响应失败: %w", endpoint, err)
	}
	return result, nil
}

func miaoyunData(result map[string]interface{}) map[string]interface{} {
	data, _ := result["data"].(map[string]interface{})
	return data
}

func miaoyunString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func miaoyunResponseError(operation string, result map[string]interface{}) error {
	code := miaoyunString(result["code"])
	failed := code != "" && code != "0" && code != "0000"
	if success, present := result["success"].(bool); present && !success {
		failed = true
	}
	if failed {
		return fmt.Errorf("%s失败: code=%s %s", operation, code, miaoyunString(result["message"]))
	}
	return nil
}

// A downstream failure is not a rejection of the selected face. Keep the
// actual business code and do not cycle through unrelated templates.
func (api *CaiyunAPI) verifyMiaoyunPhoto(userID, fileID string) (map[string]interface{}, error) {
	result, err := api.miaoyunCall("/image/verifyMainImage", map[string]interface{}{
		"userId": userID, "fileId": fileID, "imageId": "",
	})
	if err != nil {
		return nil, fmt.Errorf("AI 写真主图校验请求失败: %w", err)
	}
	data := miaoyunData(result)
	if err := miaoyunResponseError("AI 写真主图校验", result); err != nil {
		return nil, err
	}
	if message := miaoyunString(data["verifyMessage"]); message != "" {
		return result, nil
	}
	if miaoyunString(data["masterId"]) == "" || miaoyunString(data["taskId"]) == "" {
		return nil, fmt.Errorf("AI 写真主图校验未返回 masterId/taskId: %s", miaoyunString(result["message"]))
	}
	return result, nil
}

func selectMiaoyunModel(result map[string]interface{}) (interface{}, interface{}, error) {
	if err := miaoyunResponseError("查询写真模型", result); err != nil {
		return nil, nil, err
	}
	groups, _ := miaoyunData(result)["modelList"].([]interface{})
	for _, group := range groups {
		entry, _ := group.(map[string]interface{})
		items, _ := entry["list"].([]interface{})
		for _, item := range items {
			model, _ := item.(map[string]interface{})
			if value := miaoyunString(model["modelType"]); value != "" && value != "0" {
				return model["modelType"], model["poseId"], nil
			}
		}
	}
	return nil, nil, fmt.Errorf("AI 写真当前没有可用模型")
}

func (api *CaiyunAPI) ensureMiaoyunFolder(parent, name string) (string, error) {
	base := "https://personal-kd-njs.yun.139.com"
	resp, err := api.client.Post(base+"/hcy/file/create", buildUploadHeaders("10000023", ""), map[string]interface{}{
		"parentFileId": parent, "name": name, "description": "", "type": "folder", "fileRenameMode": "refuse",
	})
	if err != nil {
		return "", err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return "", err
	}
	var created struct {
		Data struct {
			FileID string `json:"fileId"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		return "", err
	}
	if created.Data.FileID != "" {
		return created.Data.FileID, nil
	}
	resp, err = api.client.Post(base+"/hcy/file/list", buildUploadHeaders("10000023", ""), map[string]interface{}{
		"fields": "starred", "orderBy": "name", "orderDirection": "DESC", "parentFileId": parent,
		"pageInfo": map[string]interface{}{"pageSize": 200, "needTotalCount": false},
	})
	if err != nil {
		return "", err
	}
	body, err = api.client.ReadResponseBody(resp)
	if err != nil {
		return "", err
	}
	var listed struct {
		Data struct {
			Items []struct {
				FileID string `json:"fileId"`
				Name   string `json:"name"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &listed); err != nil {
		return "", err
	}
	for _, item := range listed.Data.Items {
		if item.Name == name && item.FileID != "" {
			return item.FileID, nil
		}
	}
	return "", fmt.Errorf("妙云目录不存在且创建失败: %s", name)
}

// CompleteFunAIMiaoyunTask uses the authorized fixed reference portrait and the
// models currently offered by the official AI art photo service.
func (api *CaiyunAPI) CompleteFunAIMiaoyunTask() error {
	userID := strings.TrimSpace(api.client.GetUserDomainID())
	if userID == "" {
		return fmt.Errorf("AI 写真缺少 userDomainId")
	}
	parent := "/myfaverapp"
	for _, name := range []string{"AI工具箱本地上传", "AI写真", "我的形象"} {
		folder, err := api.ensureMiaoyunFolder(parent, name)
		if err != nil {
			return err
		}
		parent = folder
	}
	mainFolder, err := api.ensureMiaoyunFolder(parent, "主图")
	if err != nil {
		return err
	}
	slaveFolder, err := api.ensureMiaoyunFolder(parent, "从图")
	if err != nil {
		return err
	}
	recorded, err := api.miaoyunCall("/image/recordUnfinishFileId", map[string]interface{}{
		"userId": userID, "imageCatalogId": parent, "imageName": "我的形象",
	})
	if err != nil {
		return err
	}
	if err := miaoyunResponseError("记录写真目录", recorded); err != nil {
		return err
	}
	unfinished, err := api.miaoyunCall("/image/getUnfinishFileId", map[string]interface{}{"userId": userID})
	if err != nil {
		return err
	}
	if err := miaoyunResponseError("查询未完成写真", unfinished); err != nil {
		return err
	}
	fileAPI := NewFileAPI(api.client)
	// The nonce changes only the trailing bytes, retaining the fixed image.
	// The reference flow uses it to avoid incomplete content-hash shortcuts.
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	photo := append(append([]byte(nil), funaiReferencePortrait...), nonce...)
	uploaded, err := fileAPI.UploadRandomFile(&UploadRandomFileRequest{
		ParentFileID: mainFolder, Name: fmt.Sprintf("ai_face_%d.jpg", time.Now().UnixNano()),
		Content: photo, ContentType: "image/jpeg", ChannelSrc: "10000023", Ext: ".jpg",
	})
	if err != nil {
		return fmt.Errorf("上传写真固定人像: %w", err)
	}
	if uploaded == nil || uploaded.FileID == "" {
		return fmt.Errorf("上传写真固定人像未返回 fileId")
	}
	fileID := uploaded.FileID
	verify, err := api.verifyMiaoyunPhoto(userID, fileID)
	if err != nil {
		_, _ = fileAPI.DeleteFiles([]string{fileID})
		return err
	}
	data := miaoyunData(verify)
	masterID, taskID := miaoyunString(data["masterId"]), miaoyunString(data["taskId"])
	if masterID == "" || taskID == "" {
		_, _ = fileAPI.DeleteFiles([]string{fileID})
		return fmt.Errorf("AI 写真固定人像校验未通过: %s", miaoyunString(data["verifyMessage"]))
	}
	created, err := api.miaoyunCall("/image/createImage", map[string]interface{}{
		"userId": userID, "taskId": taskID, "fileId": fileID, "subImages": []interface{}{},
		"slaveCatalogId": slaveFolder, "imageCatalogId": parent, "imageName": "我的形象",
		"masterId": masterID, "isMy": 1, "isPermanent": 0,
	})
	if err != nil {
		return err
	}
	if err := miaoyunResponseError("创建写真形象", created); err != nil {
		return err
	}
	imageID := miaoyunString(miaoyunData(created)["imageId"])
	if imageID == "" {
		return fmt.Errorf("AI 写真未返回数字形象 ID: %v", created["message"])
	}
	models, err := api.miaoyunCall("/model/queryModelList", map[string]interface{}{"userId": userID, "imageId": imageID})
	if err != nil {
		return err
	}
	modelType, poseID, err := selectMiaoyunModel(models)
	if err != nil {
		return err
	}
	var lastArtError error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(8 * time.Second)
		}
		art, err := api.miaoyunCall("/art/autoComposeArtPhoto", map[string]interface{}{
			"userId": userID, "imageId": imageID, "modelType": modelType,
			"poseId": poseID, "numGen": 1,
		})
		if err != nil {
			return err
		}
		lastArtError = miaoyunResponseError("写真艺术照生成", art)
		if (art["success"] == true && lastArtError == nil) || strings.Contains(miaoyunString(art["message"]), "已经生成") {
			return nil
		}
	}
	if lastArtError != nil {
		return lastArtError
	}
	return fmt.Errorf("AI 写真艺术照生成请求未被受理")
}
