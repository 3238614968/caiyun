package api

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Unchanged official fallback template 1 overlay from aiTools (2026-11-12).
//
//go:embed assets/nine-grid-template1-style.png
var nineGridStyle []byte

const nineGridFileBase = "https://personal-kd-njs.yun.139.com/hcy"

var ErrNineGridRecordPending = errors.New("服务端动态列表尚未确认本次九宫格完整记录")

type NineGridResult struct {
	FolderID        string   `json:"folder_id"`
	FileIDs         []string `json:"file_ids"`
	DynamicID       string   `json:"dynamic_id,omitempty"`
	FilesVerified   bool     `json:"files_verified"`
	AuditVerified   bool     `json:"audit_verified"`
	DynamicAccepted bool     `json:"dynamic_accepted"`
	DynamicVerified bool     `json:"dynamic_verified"`
	EventAccepted   bool     `json:"event_accepted"`
}

// CompleteNineGridTask follows the current H5 flow: compose real images, save
// the cover for audit, save nine pictures, verify cloud files, then save the AI
// dynamic. Activity completion must still be checked by its caller.
func (api *CaiyunAPI) CompleteNineGridTask() (*NineGridResult, error) {
	return api.completeNineGridTask(time.Sleep)
}

func (api *CaiyunAPI) completeNineGridTask(wait func(time.Duration)) (*NineGridResult, error) {
	uid := strings.TrimSpace(api.client.GetUserDomainID())
	if uid == "" {
		return nil, fmt.Errorf("九宫格缺少账号上下文")
	}
	permission, err := api.AIStoreAccredit(15)
	if err != nil {
		return nil, err
	}
	if permission == nil || (fmt.Sprint(permission.Code) != "0" && fmt.Sprint(permission.Code) != "0000") {
		return nil, fmt.Errorf("九宫格授权查询未成功")
	}
	data, _ := permission.Data.(map[string]interface{})
	if data == nil {
		data, _ = resultMap(permission)
	}
	if data != nil {
		if nested, ok := data["data"].(map[string]interface{}); ok {
			data = nested
		}
	}
	if data == nil || fmt.Sprint(data["authStatus"]) != "1" {
		return nil, fmt.Errorf("九宫格尚未授权，请在官方入口确认授权后重试")
	}
	parent := "/"
	for _, name := range []string{"AI空间", "其他AI工具", "朋友圈9图"} {
		parent, err = api.nineGridFolder(parent, name)
		if err != nil {
			return nil, err
		}
	}
	result := &NineGridResult{FolderID: parent}
	portrait, err := jpeg.Decode(bytes.NewReader(funaiReferencePortrait))
	if err != nil {
		return result, fmt.Errorf("解码九宫格参考图片: %w", err)
	}
	style, err := png.Decode(bytes.NewReader(nineGridStyle))
	if err != nil {
		return result, fmt.Errorf("解码九宫格官方模板: %w", err)
	}
	inputs := make([]image.Image, 9)
	for i := range inputs {
		inputs[i] = portrait
	}
	cover, tiles, err := renderNineGrid(inputs, style)
	if err != nil {
		return result, err
	}
	stamp := time.Now().In(time.FixedZone("CST", 8*3600)).Format("20060102-150405") + "-" + uuid.NewString()
	files := NewFileAPI(api.client)
	var infos []map[string]string
	upload := func(name string, content []byte, generated bool) (string, error) {
		req := &UploadRandomFileRequest{ParentFileID: parent, Name: name, Content: content, ContentType: "image/jpeg", ChannelSrc: "101"}
		if generated {
			req.OpType = "aistore"
			req.TagList = []UploadFileTag{{Key: "sub_biz_type", Value: "100003"}, {Key: "biz_type", Value: "100"}}
		}
		file, e := files.UploadRandomFile(req)
		if e != nil {
			return "", e
		}
		if file == nil || file.FileID == "" {
			return "", fmt.Errorf("九宫格上传缺少 fileId")
		}
		result.FileIDs = append(result.FileIDs, file.FileID)
		return file.FileID, nil
	}
	coverName := "ninegrid-" + stamp + "-cover.jpg"
	coverID, err := upload(coverName, cover, true)
	if err != nil {
		return result, err
	}
	// Native audit status: 0 processing, 2 passed, 3 rejected. Do not publish
	// a dynamic or call the activity complete after an unconfirmed audit.
	for attempt := 0; attempt < 30; attempt++ {
		if attempt > 0 {
			wait(time.Second)
		}
		data, e := api.nineGridRequest("/file/getAuditResult", map[string]string{"fileId": coverID})
		if e != nil {
			return result, e
		}
		var audit struct {
			ContentAuditResult *struct {
				Status *int `json:"status"`
			} `json:"contentAuditResult"`
		}
		if json.Unmarshal(data, &audit) != nil || audit.ContentAuditResult == nil || audit.ContentAuditResult.Status == nil {
			return result, fmt.Errorf("九宫格审核结果缺失")
		}
		switch *audit.ContentAuditResult.Status {
		case 0:
			continue
		case 2:
			result.AuditVerified = true
		default:
			return result, fmt.Errorf("九宫格审核未通过，状态=%d", *audit.ContentAuditResult.Status)
		}
		break
	}
	if !result.AuditVerified {
		return result, fmt.Errorf("九宫格封面审核仍在处理中")
	}
	infos = append(infos, map[string]string{"fileId": coverID, "fileName": coverName, "parentFileId": parent})
	for i, tile := range tiles {
		name := fmt.Sprintf("ninegrid-%s-%d.jpg", stamp, i+1)
		id, e := upload(name, tile, true)
		if e != nil {
			return result, e
		}
		infos = append(infos, map[string]string{"fileId": id, "fileName": name, "parentFileId": parent})
	}
	sourceName := "ninegrid-" + stamp + "-source.jpg"
	sourceID, err := upload(sourceName, funaiReferencePortrait, false)
	if err != nil {
		return result, err
	}
	if err := api.verifyNineGridFiles(result.FileIDs); err != nil {
		return result, err
	}
	result.FilesVerified = true
	params, _ := json.Marshal(map[string]interface{}{"module": 15, "templateId": 1, "name": "朋友圈9图", "multipleNum": 9})
	paramText, _ := json.Marshal(map[string]string{"type": "朋友圈9图", "paramText": "模版：朋友圈9图", "prompt": ""})
	body := map[string]interface{}{"userId": uid, "sourceChannel": "101", "dynamicInfo": map[string]interface{}{
		"sessionId": uuid.NewString(), "title": "朋友圈9图", "param": string(params), "dynamicSource": "aiTool", "dynamicTime": time.Now().UTC().Format(time.RFC3339Nano),
		"subFeature": "momentsNineGrid", "sourceFileInfos": []map[string]string{{"fileId": sourceID, "fileName": sourceName, "parentFileId": parent}},
		"resultFileInfos": infos, "productLocationParentId": parent, "fileTypes": []string{"image"}, "paramText": string(paramText),
	}}
	resp, err := api.client.Post(AIYunURL+"/api/dynamic/chat/save", api.funaiImageHeaders(), body)
	if err != nil {
		return result, err
	}
	text, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return result, err
	}
	if err := validateFileUploadResponse(resp, text); err != nil {
		return result, fmt.Errorf("九宫格动态保存: %w", err)
	}
	var saved struct {
		Data struct {
			DynamicID string `json:"dynamicId"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &saved); err != nil {
		return result, err
	}
	result.DynamicID = saved.Data.DynamicID
	result.DynamicAccepted = true
	if err := api.waitNineGridRecord(result.DynamicID, wait); err != nil {
		return result, err
	}
	result.DynamicVerified = true
	if err := api.reportNineGridSaved(result, time.Now()); err != nil {
		return result, err
	}
	result.EventAccepted = true
	return result, nil
}

// VerifyNineGridRecord reads the native v3 dynamic list. A save acknowledgement
// is not enough; the exact generated ID must have ten persisted result files.
func (api *CaiyunAPI) VerifyNineGridRecord(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("九宫格动态保存缺少记录ID")
	}
	headers := api.funaiImageHeaders()
	headers["x-yun-api-version"] = "v3"
	var cursor interface{}
	seen := map[string]bool{}
	for page := 0; page < 5; page++ {
		resp, err := api.client.Post(AIYunURL+"/api/dynamic/chat/list", headers, map[string]interface{}{
			"userId": api.client.GetUserDomainID(), "sourceChannel": "101",
			"pageInfo": map[string]interface{}{"pageSize": 30, "pageCursor": cursor, "needTotalCount": 0},
		})
		if err != nil {
			return err
		}
		text, err := api.client.ReadResponseBody(resp)
		if err != nil {
			return err
		}
		if err := validateFileUploadResponse(resp, text); err != nil {
			return fmt.Errorf("九宫格动态复查: %w", err)
		}
		var records struct {
			Data struct {
				List []struct {
					ID        string `json:"dynamicId"`
					Title     string `json:"title"`
					FileCount int    `json:"fileCount"`
				} `json:"list"`
				Next string `json:"nextPageCursor"`
			} `json:"data"`
		}
		if json.Unmarshal([]byte(text), &records) != nil || records.Data.List == nil {
			return fmt.Errorf("九宫格动态列表无效")
		}
		for _, record := range records.Data.List {
			if record.ID == id {
				if record.Title != "朋友圈9图" {
					return fmt.Errorf("九宫格动态记录的作品不完整")
				}
				if record.FileCount != 10 {
					return ErrNineGridRecordPending
				}
				return nil
			}
		}
		if records.Data.Next == "" || seen[records.Data.Next] {
			break
		}
		seen[records.Data.Next] = true
		cursor = records.Data.Next
	}
	return ErrNineGridRecordPending
}

func (api *CaiyunAPI) waitNineGridRecord(id string, wait func(time.Duration)) error {
	for attempt := 0; attempt < 6; attempt++ {
		if attempt > 0 {
			wait(3 * time.Second)
		}
		err := api.VerifyNineGridRecord(id)
		if !errors.Is(err, ErrNineGridRecordPending) || attempt == 5 {
			return err
		}
	}
	return ErrNineGridRecordPending
}

func (api *CaiyunAPI) nineGridRequest(path string, body interface{}) (json.RawMessage, error) {
	headers := api.funaiImageHeaders()
	headers["x-yun-op-type"], headers["x-yun-sub-op-type"] = "1", "109"
	resp, err := api.client.Post(nineGridFileBase+path, headers, body)
	if err != nil {
		return nil, err
	}
	text, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if err := validateFileUploadResponse(resp, text); err != nil {
		return nil, fmt.Errorf("九宫格 %s: %w", path, err)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		return nil, err
	}
	return envelope.Data, nil
}

func (api *CaiyunAPI) nineGridFolder(parent, name string) (string, error) {
	// The native H5 uses refuse; the service returns the existing folder when
	// present. Never use auto_rename to create another AI-space tree on retries.
	data, err := api.nineGridRequest("/file/create", map[string]string{"parentFileId": parent, "name": name, "type": "folder", "fileRenameMode": "refuse"})
	if err != nil {
		return "", err
	}
	var folder struct {
		FileID string `json:"fileId"`
	}
	if json.Unmarshal(data, &folder) != nil || folder.FileID == "" {
		return "", fmt.Errorf("九宫格目录创建未返回 fileId")
	}
	return folder.FileID, nil
}

func (api *CaiyunAPI) verifyNineGridFiles(ids []string) error {
	data, err := api.nineGridRequest("/file/batchGet", map[string]interface{}{"fileIds": ids})
	if err != nil {
		return err
	}
	var batch struct {
		BatchFileResults []struct {
			SrcFile struct {
				FileID string `json:"fileId"`
				Size   int64  `json:"size"`
			} `json:"srcFile"`
		} `json:"batchFileResults"`
	}
	if err := json.Unmarshal(data, &batch); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, file := range batch.BatchFileResults {
		if file.SrcFile.Size > 0 {
			seen[file.SrcFile.FileID] = true
		}
	}
	for _, id := range ids {
		if !seen[id] {
			return fmt.Errorf("九宫格云端文件尚未确认")
		}
	}
	return nil
}

// The official fallback template center-crops each input to a square, overlays
// template1_style, and makes a 1080px cover (8/7/115 units on a 367-unit grid).
func renderNineGrid(inputs []image.Image, style image.Image) ([]byte, [][]byte, error) {
	if len(inputs) != 9 || style == nil {
		return nil, nil, fmt.Errorf("九宫格需要九张有效图片和模板")
	}
	const size = 1080
	cover := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(cover, cover.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	tiles := make([][]byte, 0, 9)
	for i, src := range inputs {
		if src == nil || src.Bounds().Empty() {
			return nil, nil, fmt.Errorf("九宫格输入图片无效")
		}
		tile := image.NewRGBA(image.Rect(0, 0, size, size))
		scaleNineGrid(tile, tile.Bounds(), src, true, draw.Src)
		scaleNineGrid(tile, tile.Bounds(), style, false, draw.Over)
		var encoded bytes.Buffer
		if err := jpeg.Encode(&encoded, tile, &jpeg.Options{Quality: 95}); err != nil {
			return nil, nil, err
		}
		tiles = append(tiles, encoded.Bytes())
		x, y := 24+(i%3)*359, 24+(i/3)*359
		scaleNineGrid(cover, image.Rect(x, y, x+338, y+338), tile, false, draw.Src)
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, cover, &jpeg.Options{Quality: 95}); err != nil {
		return nil, nil, err
	}
	return encoded.Bytes(), tiles, nil
}

func scaleNineGrid(dst *image.RGBA, rect image.Rectangle, src image.Image, crop bool, op draw.Op) {
	b := src.Bounds()
	if crop {
		side := min(b.Dx(), b.Dy())
		b = image.Rect(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2, b.Min.X+(b.Dx()-side)/2+side, b.Min.Y+(b.Dy()-side)/2+side)
	}
	scaled := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			scaled.Set(x, y, src.At(b.Min.X+x*b.Dx()/rect.Dx(), b.Min.Y+y*b.Dy()/rect.Dy()))
		}
	}
	draw.Draw(dst, rect, scaled, image.Point{}, op)
}
