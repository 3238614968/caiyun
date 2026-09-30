package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"time"
)

const posterUploadMarketName = "National_PosterMaking"

func generateTaskPosterPNG() ([]byte, error) {
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	const size = 512
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(70 + x/5),
				G: uint8(125 + y/6),
				B: uint8(160 + (x+y)/12),
				A: 255,
			})
		}
	}
	// A small unique accent makes each generated poster a fresh upload.
	for y := 230; y < 282; y++ {
		for x := 230; x < 282; x++ {
			img.SetRGBA(x, y, color.RGBA{R: nonce[0], G: nonce[1], B: nonce[2], A: 255})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// CompletePosterTask uploads a generated PNG through the event's presigned
// object-storage flow. The task list uses a different market name from upload.
func (api *CaiyunAPI) CompletePosterTask() error {
	content, err := generateTaskPosterPNG()
	if err != nil {
		return err
	}
	name := fmt.Sprintf("poster_%d.png", time.Now().UnixNano())
	body, err := api.activityPostBody(PosterMarketName, MobileMarketURL+"/api/cloud/ose/activity/getUploadUrl", map[string]interface{}{
		"marketName": posterUploadMarketName,
		"fileName":   name,
		"fileSize":   len(content),
	})
	if err != nil {
		return err
	}
	envelope, err := decodeActivityBody("获取校园海报上传地址", body)
	if err != nil {
		return err
	}
	var reservation struct {
		UploadURL     string `json:"uploadUrl"`
		UploadID      string `json:"uploadId"`
		FileID        string `json:"fileId"`
		HashAlgorithm string `json:"hashAlgorithm"`
	}
	if err := json.Unmarshal(envelope.Result, &reservation); err != nil {
		return fmt.Errorf("解析校园海报上传地址失败: %w", err)
	}
	if reservation.UploadURL == "" || reservation.UploadID == "" || reservation.FileID == "" {
		return fmt.Errorf("校园海报上传地址缺少 URL、uploadId 或 fileId")
	}
	if reservation.HashAlgorithm == "" {
		reservation.HashAlgorithm = "SHA256"
	}
	if reservation.HashAlgorithm != "SHA256" {
		return fmt.Errorf("不支持的海报内容哈希算法: %s", reservation.HashAlgorithm)
	}
	uploadResp, err := api.client.PutPresigned(reservation.UploadURL, "image/png", content)
	if err != nil {
		return fmt.Errorf("上传校园海报内容失败: %w", err)
	}
	if _, err := api.client.ReadResponseBody(uploadResp); err != nil {
		return err
	}
	if uploadResp.StatusCode < 200 || uploadResp.StatusCode >= 300 {
		return fmt.Errorf("上传校园海报 HTTP %d", uploadResp.StatusCode)
	}
	hash := sha256.Sum256(content)
	completed, err := api.activityPostBody(PosterMarketName, MobileMarketURL+"/api/cloud/ose/file/complete", map[string]interface{}{
		"uploadId":             reservation.UploadID,
		"fileId":               reservation.FileID,
		"contentHash":          hex.EncodeToString(hash[:]),
		"contentHashAlgorithm": reservation.HashAlgorithm,
	})
	if err != nil {
		return err
	}
	completeEnvelope, err := decodeActivityBody("完成校园海报上传", completed)
	if err != nil {
		return err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(completeEnvelope.Result, &result); err != nil {
		return err
	}
	if !boolFromAny(result["isAddLottoryCnt"]) {
		return fmt.Errorf("海报已上传，但服务端未确认增加抽奖机会")
	}
	return nil
}
