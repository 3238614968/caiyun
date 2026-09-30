package api

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"strings"
	"time"
)

const miaoyunBaseURL = AIYunURL + "/aitools/miaoyun"

func (api *CaiyunAPI) miaoyunCall(endpoint string, payload interface{}) (map[string]interface{}, error) {
	resp, err := api.client.Post(miaoyunBaseURL+endpoint, map[string]string{
		"Content-Type": "application/json;charset:utf-8;",
	}, payload)
	if err != nil {
		return nil, err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
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

// CompleteFunAIMiaoyunTask creates a digital portrait from a public vendor
// template, then requests the official AI art photo operation.
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
	_, _ = api.miaoyunCall("/image/recordUnfinishFileId", map[string]interface{}{
		"userId": userID, "imageCatalogId": parent, "imageName": "我的形象",
	})
	templates, err := api.funaiAdTemplates("66391", nil)
	if err != nil {
		return err
	}
	fileAPI := NewFileAPI(api.client)
	var rejectedFiles []string
	defer func() {
		if len(rejectedFiles) > 0 {
			_, _ = fileAPI.DeleteFiles(rejectedFiles)
		}
	}()
	var fileID, masterID, taskID string
	for _, template := range templates[:min(len(templates), 12)] {
		materialURL := funaiMaterialURL(template)
		if materialURL == "" {
			continue
		}
		photo, err := api.client.GetPublicBytes(materialURL, 10<<20)
		if err != nil || len(photo) < 1000 {
			continue
		}
		picture, _, err := image.Decode(bytes.NewReader(photo))
		if err != nil || picture.Bounds().Dx() < 500 || picture.Bounds().Dy() < 500 {
			continue
		}
		var jpegPhoto bytes.Buffer
		if err := jpeg.Encode(&jpegPhoto, picture, &jpeg.Options{Quality: 90}); err != nil {
			continue
		}
		// A unique content hash avoids the cloud upload shortcut returning no uploadId.
		nonce := make([]byte, 8)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		photo = append(jpegPhoto.Bytes(), nonce...)
		uploaded, err := fileAPI.UploadRandomFile(&UploadRandomFileRequest{
			ParentFileID: mainFolder, Name: fmt.Sprintf("ai_face_%d.jpg", time.Now().UnixNano()),
			Content: photo, ContentType: "image/jpeg", ChannelSrc: "10000023", Ext: ".jpg",
		})
		if err != nil || uploaded == nil || uploaded.FileID == "" {
			continue
		}
		verify, err := api.miaoyunCall("/image/verifyMainImage", map[string]interface{}{
			"userId": userID, "fileId": uploaded.FileID, "imageId": "",
		})
		if err != nil {
			rejectedFiles = append(rejectedFiles, uploaded.FileID)
			continue
		}
		data := miaoyunData(verify)
		if id := miaoyunString(data["masterId"]); id != "" {
			fileID, masterID, taskID = uploaded.FileID, id, miaoyunString(data["taskId"])
			break
		}
		rejectedFiles = append(rejectedFiles, uploaded.FileID)
	}
	if fileID == "" || masterID == "" || taskID == "" {
		return fmt.Errorf("AI 写真没有通过正脸校验的模板图")
	}
	created, err := api.miaoyunCall("/image/createImage", map[string]interface{}{
		"userId": userID, "taskId": taskID, "fileId": fileID, "subImages": []interface{}{},
		"slaveCatalogId": slaveFolder, "imageCatalogId": parent, "imageName": "我的形象",
		"masterId": masterID, "isMy": 1, "isPermanent": 0,
	})
	if err != nil {
		return err
	}
	imageID := miaoyunString(miaoyunData(created)["imageId"])
	if imageID == "" {
		return fmt.Errorf("AI 写真未返回数字形象 ID: %v", created["message"])
	}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(8 * time.Second)
		}
		art, err := api.miaoyunCall("/art/autoComposeArtPhoto", map[string]interface{}{
			"userId": userID, "imageId": imageID, "modelType": 1001,
			"poseId": "", "numGen": 1,
		})
		if err != nil {
			return err
		}
		if art["success"] == true || strings.Contains(miaoyunString(art["message"]), "已经生成") {
			return nil
		}
	}
	return fmt.Errorf("AI 写真艺术照生成请求未被受理")
}
