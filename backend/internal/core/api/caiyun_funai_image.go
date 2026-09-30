package api

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed assets/ai-task-portrait.png
var funaiSyntheticPortrait []byte

var funaiPortraitCache struct {
	sync.Once
	jpegBase64 string
	maskBase64 string
	err        error
}

func funaiImageInputs() (portrait, mask string, err error) {
	funaiPortraitCache.Do(func() {
		img, decodeErr := png.Decode(bytes.NewReader(funaiSyntheticPortrait))
		if decodeErr != nil {
			funaiPortraitCache.err = fmt.Errorf("解码内置合成人像失败: %w", decodeErr)
			return
		}
		var jpegBytes bytes.Buffer
		if encodeErr := jpeg.Encode(&jpegBytes, img, &jpeg.Options{Quality: 86}); encodeErr != nil {
			funaiPortraitCache.err = encodeErr
			return
		}
		bounds := img.Bounds()
		white := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				white.Set(x, y, color.White)
			}
		}
		var maskBytes bytes.Buffer
		if encodeErr := png.Encode(&maskBytes, white); encodeErr != nil {
			funaiPortraitCache.err = encodeErr
			return
		}
		funaiPortraitCache.jpegBase64 = base64.StdEncoding.EncodeToString(jpegBytes.Bytes())
		funaiPortraitCache.maskBase64 = base64.StdEncoding.EncodeToString(maskBytes.Bytes())
	})
	return funaiPortraitCache.jpegBase64, funaiPortraitCache.maskBase64, funaiPortraitCache.err
}

func (api *CaiyunAPI) funaiImageHeaders() map[string]string {
	headers := api.buildAIHeaders(false)
	headers["x-yun-app-channel"] = "101"
	headers["Origin"] = "https://yun.139.com"
	headers["Referer"] = "https://yun.139.com/aiTools/"
	if userID := strings.TrimSpace(api.client.GetUserDomainID()); userID != "" {
		headers["x-yun-uni"] = userID
		headers["Cookie"] = "userDomainId=" + userID
	}
	return headers
}

func (api *CaiyunAPI) funaiAdTemplates(slot string, extra map[string]interface{}) ([]map[string]interface{}, error) {
	payload := map[string]interface{}{"adpostid": slot}
	for key, value := range extra {
		payload[key] = value
	}
	resp, err := api.client.Post(
		"https://ad.mcloud.139.com/advertapi/adv-config/adv-config/AdInfoFilter/getAdInfos",
		api.funaiImageHeaders(), payload,
	)
	if err != nil {
		return nil, err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}
	var result struct {
		ReturnCode interface{}              `json:"returnCode"`
		Body       []map[string]interface{} `json:"body"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	if fmt.Sprint(result.ReturnCode) != "0" || len(result.Body) == 0 {
		return nil, fmt.Errorf("AI 模板列表不可用: code=%v", result.ReturnCode)
	}
	return result.Body, nil
}

func funaiTemplateString(template map[string]interface{}, key string) string {
	value := template[key]
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func funaiMaterialURL(template map[string]interface{}) string {
	materials, ok := template["materialList"].([]interface{})
	if !ok || len(materials) == 0 {
		return ""
	}
	first, ok := materials[0].(map[string]interface{})
	if !ok {
		return ""
	}
	return funaiTemplateString(first, "materialUrl")
}

func (api *CaiyunAPI) funaiSingleFace(portrait string) (bool, error) {
	resp, err := api.client.Post(AIYunURL+"/api/image/face/analysis", api.funaiImageHeaders(), map[string]interface{}{
		"sendType": 2, "channelId": "100102", "imageExt": "jpg", "base64": portrait, "supplierType": 8,
	})
	if err != nil {
		return false, err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return false, err
	}
	var result struct {
		Code interface{} `json:"code"`
		Data struct {
			ExistFace interface{} `json:"existFace"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return false, err
	}
	if fmt.Sprint(result.Code) != "0000" && fmt.Sprint(result.Code) != "0" {
		return false, fmt.Errorf("人脸分析失败: code=%v", result.Code)
	}
	return boolFromAny(result.Data.ExistFace), nil
}

func (api *CaiyunAPI) funaiImageTaskStatus(taskID string) (int, error) {
	for attempt := 0; attempt < 8; attempt++ {
		if attempt > 0 {
			time.Sleep(4 * time.Second)
		}
		resp, err := api.client.Post(AIYunURL+"/api/outer/async/task/result", api.funaiImageHeaders(), map[string]string{"taskId": taskID})
		if err != nil {
			return 0, err
		}
		body, err := api.client.ReadResponseBody(resp)
		if err != nil {
			return 0, err
		}
		var result struct {
			Data struct {
				Status int `json:"status"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			return 0, err
		}
		if result.Data.Status == 3 || result.Data.Status == 4 {
			return result.Data.Status, nil
		}
	}
	return 0, fmt.Errorf("AI 图像任务处理超时")
}

// CompleteFunAIImageTask runs an image algorithm only for known task IDs.
// This confirms submission only. Activity completion must be checked in the
// task table: it can be credited even if rendering later fails (notably 031).
func (api *CaiyunAPI) CompleteFunAIImageTask(taskID string) error {
	if strings.TrimSpace(api.client.GetUserDomainID()) == "" {
		return fmt.Errorf("趣玩 AI 图像任务缺少 userDomainId")
	}
	portrait, mask, err := funaiImageInputs()
	if err != nil {
		return err
	}
	base := map[string]interface{}{
		"sendType": 2, "channelId": "100102", "imageExt": "jpg", "base64": portrait, "supplierType": 8,
	}
	endpoint := ""
	var payload interface{} = base
	switch taskID {
	case "003":
		endpoint = "/api/image/edit/portraitMatting"
		base["supplierType"] = 4
	case "006":
		endpoint = "/api/image/edit/erasure"
		base["maskBase64"] = mask
	case "009":
		endpoint = "/api/image/edit/qualityRestore"
	case "007":
		endpoint = "/api/image/avatar/cartoon"
		base["supplierType"] = 4
		base["style"] = "116"
	case "008":
		endpoint = "/api/image/edit/restore"
		base["style"] = 1
	case "004":
		endpoint = "/api/image/edit/faceAnime"
		base["supplierType"] = 4
		base["style"] = "107"
		base["addWatermark"] = true
	case "3Drenou":
		endpoint = "/api/image/edit/faceAnime"
		base["supplierType"] = 7
		base["style"] = "plastic_bubble_figure"
		base["addWatermark"] = true
	case "034":
		endpoint = "/api/image/faceswap"
		templates, err := api.funaiAdTemplates("66391", nil)
		if err != nil {
			return err
		}
		var template map[string]interface{}
		var style int
		for _, candidate := range templates {
			if parsed, parseErr := strconv.Atoi(funaiTemplateString(candidate, "templateId")); parseErr == nil && funaiMaterialURL(candidate) != "" {
				template, style = candidate, parsed
				break
			}
		}
		if template == nil {
			return fmt.Errorf("AI 快照模板缺少有效 id 或图片")
		}
		ext, _ := json.Marshal(map[string]string{"templateImg": funaiMaterialURL(template)})
		payload = map[string]interface{}{
			"personList": []interface{}{map[string]interface{}{
				"sendType": 2, "channelId": "100102", "imageExt": "jpg", "base64": portrait,
			}}, "channelId": "100102", "supplierType": 0,
			"style": style, "poseIdList": []int{0}, "extJson": string(ext),
		}
	case "031":
		endpoint = "/api/image/emo"
		hasFace, err := api.funaiSingleFace(portrait)
		if err != nil || !hasFace {
			return fmt.Errorf("合成人像未通过单人脸分析: %v", err)
		}
		templates, err := api.funaiAdTemplates("66344", map[string]interface{}{"client": "wap"})
		if err != nil {
			return err
		}
		var template map[string]interface{}
		for _, candidate := range templates {
			if funaiTemplateString(candidate, "templateSamp") != "" && funaiTemplateString(candidate, "templatePic") != "" {
				template = candidate
				break
			}
		}
		if template == nil {
			return fmt.Errorf("AI 表情包模板缺少视频或贴纸地址")
		}
		mediaMode, _ := strconv.Atoi(funaiTemplateString(template, "mediaMode"))
		base["templateUrl"] = funaiTemplateString(template, "templateSamp")
		base["stickerUrl"] = funaiTemplateString(template, "templatePic")
		base["templateId"] = template["templateId"]
		base["animalFlag"] = 0
		base["mediaMode"] = mediaMode
		base["stickerText"] = ""
	default:
		return fmt.Errorf("不支持的趣玩 AI 图像任务: %s", taskID)
	}
	resp, err := api.client.Post(AIYunURL+endpoint, api.funaiImageHeaders(), payload)
	if err != nil {
		return err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	var result struct {
		Code interface{} `json:"code"`
		Data struct {
			TaskID json.RawMessage `json:"taskId"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return err
	}
	if fmt.Sprint(result.Code) != "0000" && fmt.Sprint(result.Code) != "0" {
		return fmt.Errorf("趣玩 AI 图像任务 %s 创建失败: code=%v", taskID, result.Code)
	}
	asyncID := strings.Trim(strings.TrimSpace(string(result.Data.TaskID)), `"`)
	if asyncID == "" || asyncID == "null" {
		return fmt.Errorf("趣玩 AI 图像任务 %s 未返回异步任务 ID", taskID)
	}
	return nil
}
