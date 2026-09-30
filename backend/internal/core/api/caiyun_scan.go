package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

const scanTransportKey = "2olBaQGYnEKoStYomsd1n7ax"

func encryptScanTransport(plaintext []byte) (string, error) {
	block, err := aes.NewCipher([]byte(scanTransportKey))
	if err != nil {
		return "", err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	pad := aes.BlockSize - len(plaintext)%aes.BlockSize
	for i := 0; i < pad; i++ {
		plaintext = append(plaintext, byte(pad))
	}
	ciphertext := make([]byte, len(plaintext))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, plaintext)
	return base64.StdEncoding.EncodeToString(append(iv, ciphertext...)), nil
}

func decryptScanTransport(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, err
	}
	if len(raw) < aes.BlockSize*2 || len(raw)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("扫描文档响应密文长度无效")
	}
	block, err := aes.NewCipher([]byte(scanTransportKey))
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(raw)-aes.BlockSize)
	cipher.NewCBCDecrypter(block, raw[:aes.BlockSize]).CryptBlocks(plaintext, raw[aes.BlockSize:])
	pad := int(plaintext[len(plaintext)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(plaintext) {
		return nil, fmt.Errorf("扫描文档响应填充无效")
	}
	for _, value := range plaintext[len(plaintext)-pad:] {
		if int(value) != pad {
			return nil, fmt.Errorf("扫描文档响应填充无效")
		}
	}
	return plaintext[:len(plaintext)-pad], nil
}

func (api *CaiyunAPI) scanHeaders(save bool) map[string]string {
	headers := api.funaiImageHeaders()
	headers["x-yun-app-channel"] = "10000023"
	headers["x-yun-user-agent"] = "android|23049RAD8C|android 13|mCloud13.2.2-032"
	headers["x-NetType"] = "1"
	headers["x-MM-Source"] = "032"
	headers["x-SvcType"] = "1"
	headers["User-Agent"] = "okhttp/4.12.0"
	if save {
		delete(headers, "x-yun-api-version")
		headers["X-YUN-API-VERSION"] = "3"
		headers["hcy-cool-flag"] = "1"
		headers["Content-Type"] = "application/json; charset=UTF-8"
	}
	return headers
}

// CompleteFunAIScanTask sends a real image through the camera algorithm and
// saves the processed URL through the adaptor's encrypted transport.
func (api *CaiyunAPI) CompleteFunAIScanTask() error {
	userID := strings.TrimSpace(api.client.GetUserDomainID())
	if userID == "" {
		return fmt.Errorf("扫描文档缺少 userDomainId")
	}
	portrait, _, err := funaiImageInputs()
	if err != nil {
		return err
	}
	request := map[string]interface{}{
		"userId": userID, "channelId": "100101", "algorithmNames": []string{"PhotoEnhance"},
		"angleDetection": false, "dewarp": false, "base64": portrait,
		"fileId": nil, "fileUrl": nil, "fileName": fmt.Sprintf("扫描文档_%d", time.Now().UnixMilli()),
		"supplierType": 7, "sendType": 2, "imageExt": "jpg", "uploadToCloud": true,
	}
	resp, err := api.client.Post(AIYunURL+"/api/image/aiCamera/algorithmLayout", api.scanHeaders(false), request)
	if err != nil {
		return err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	var created struct {
		Code interface{} `json:"code"`
		Data struct {
			TaskID json.RawMessage `json:"taskId"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		return err
	}
	taskID := strings.Trim(strings.TrimSpace(string(created.Data.TaskID)), `"`)
	if (fmt.Sprint(created.Code) != "0000" && fmt.Sprint(created.Code) != "0") || taskID == "" || taskID == "null" {
		return fmt.Errorf("扫描文档算法未返回任务 ID: code=%v", created.Code)
	}

	fileURL := ""
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			time.Sleep(6 * time.Second)
		}
		resp, err := api.client.Post(AIYunURL+"/api/outer/async/task/resultList", api.scanHeaders(false), map[string]interface{}{
			"taskIds": []string{taskID}, "userId": userID,
		})
		if err != nil {
			return err
		}
		body, err := api.client.ReadResponseBody(resp)
		if err != nil {
			return err
		}
		var result struct {
			Data []struct {
				ResultList []struct {
					FileURLList []string `json:"fileUrlList"`
				} `json:"resultList"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			return err
		}
		for _, batch := range result.Data {
			for _, item := range batch.ResultList {
				if len(item.FileURLList) > 0 {
					fileURL = item.FileURLList[0]
					break
				}
			}
		}
		if fileURL != "" {
			break
		}
	}
	if fileURL == "" {
		return fmt.Errorf("扫描文档算法未返回图片地址")
	}
	saveBody, err := json.Marshal(map[string]interface{}{
		"scanType": 1, "fileInfoList": []map[string]interface{}{{"order": 1, "picFileUrl": fileURL}},
	})
	if err != nil {
		return err
	}
	encrypted, err := encryptScanTransport(saveBody)
	if err != nil {
		return err
	}
	resp, err = api.client.Post("https://orches.yun.139.com/adaptor/ai-camera/scans/pic/save", api.scanHeaders(true), encrypted)
	if err != nil {
		return err
	}
	body, err = api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "{") {
		decoded, decryptErr := decryptScanTransport(body)
		if decryptErr != nil {
			return decryptErr
		}
		body = string(decoded)
	}
	var saved struct {
		Code interface{} `json:"code"`
		Data struct {
			ScanRecordID interface{} `json:"scanRecordId"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &saved); err != nil {
		return err
	}
	if fmt.Sprint(saved.Code) != "0" && fmt.Sprint(saved.Code) != "0000" {
		return fmt.Errorf("扫描文档保存失败: code=%v", saved.Code)
	}
	return nil
}
