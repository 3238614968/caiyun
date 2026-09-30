package api

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"
)

// middle.yun.139.com/openapi/*（caixun 营销平台）。
//
// 这一族与云盘/活动层不是同一套鉴权：只带 Authorization 会回
// `401 请求参数不完整`，必须补齐平台自有的整套头
// （authType / clientId / msgId / timeStamp / signature / version / x-DeviceInfo）。
//
// 本文件只承载「相册自动备份开关状态上报」这一条链路。该端点的语义是
// **客户端上报本机真实开关值**，因此 status 由调用方传入，本实现不提供
// 「强制置 1」的入口 —— 在未真正开启备份时提交 status=1 属于向活动方
// 提交虚假状态。
const caixunOpenAPIBase = "https://middle.yun.139.com/openapi"

// caixunClientKeyRelease 对应 APK 的 CaixunConstant.CLIENT_KEY_RELEASE（AES-128）。
const caixunClientKeyRelease = "nKdf317moJiFTrNq"

// AlbumAutoBackupStatus 查询服务端记录的相册备份状态。
func (api *CaiyunAPI) AlbumAutoBackupStatus() (*CaiyunResponse, error) {
	return api.caixunOpenAPIRequest("POST", "/albumAutoBackup/getStatus", map[string]interface{}{})
}

// ReportAlbumAutoBackupStatus 上报本机相册自动备份开关的真实状态。
// status 必须是设备实际值：1 已开启、0 未开启。
func (api *CaiyunAPI) ReportAlbumAutoBackupStatus(phone string, status int) (*CaiyunResponse, error) {
	if status != 0 && status != 1 {
		return nil, fmt.Errorf("无效的备份开关状态: %d", status)
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil, fmt.Errorf("上报备份状态需要手机号")
	}
	encoded, err := EncryptPhoneWithIV(caixunClientKeyRelease, phone)
	if err != nil {
		return nil, err
	}
	return api.caixunOpenAPIRequest("POST", "/albumAutoBackup/synStatus", map[string]interface{}{
		"encodeType": "2",
		"encodeData": encoded,
		"status":     status,
	})
}

// EncryptPhoneWithIV 等价于 APK 的 AESUtil.encryptIV：base64(IV‖AES-CBC/PKCS5)。
func EncryptPhoneWithIV(key, plaintext string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", fmt.Errorf("初始化 AES 失败: %w", err)
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("生成 IV 失败: %w", err)
	}
	data := []byte(plaintext)
	padding := aes.BlockSize - len(data)%aes.BlockSize
	for i := 0; i < padding; i++ {
		data = append(data, byte(padding))
	}
	ciphertext := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, data)
	return base64.StdEncoding.EncodeToString(append(iv, ciphertext...)), nil
}

// DecryptWithIV 解密 EncryptPhoneWithIV 的产物，供校验/测试使用。
func DecryptWithIV(key, encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("密文不是合法 base64: %w", err)
	}
	if len(raw) < aes.BlockSize*2 || len(raw)%aes.BlockSize != 0 {
		return "", fmt.Errorf("密文长度无效")
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", fmt.Errorf("初始化 AES 失败: %w", err)
	}
	plaintext := make([]byte, len(raw)-aes.BlockSize)
	cipher.NewCBCDecrypter(block, raw[:aes.BlockSize]).CryptBlocks(plaintext, raw[aes.BlockSize:])
	padding := int(plaintext[len(plaintext)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(plaintext) {
		return "", fmt.Errorf("填充无效")
	}
	for _, value := range plaintext[len(plaintext)-padding:] {
		if int(value) != padding {
			return "", fmt.Errorf("填充无效")
		}
	}
	return string(plaintext[:len(plaintext)-padding]), nil
}

func (api *CaiyunAPI) caixunOpenAPIRequest(method, path string, payload interface{}) (*CaiyunResponse, error) {
	reqID, err := newActivityUUID()
	if err != nil {
		return nil, err
	}
	deviceID, err := newActivityUUID()
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"authType":         "1",
		"clientId":         "hcymakeapp",
		"msgId":            reqID,
		"x-yun-tid":        reqID,
		"signature":        "null",
		"timeStamp":        fmt.Sprintf("%d", time.Now().UnixMilli()),
		"version":          "V1.1",
		"Content-Type":     "application/json",
		"Accept":           "*/*",
		"Origin":           "https://yun.139.com",
		"Referer":          "https://yun.139.com/",
		"X-Requested-With": "com.chinamobile.mcloud",
		"x-DeviceInfo":     "||36|13.2.2||23049RAD8C|" + deviceID + "||android 13|||||",
		"User-Agent":       "Mozilla/5.0 (Linux; Android 13; 23049RAD8C) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/108.0.5359.128 Mobile Safari/537.36 MCloudApp/13.2.2 tid/" + reqID + " AIModeHome",
	}
	resp, err := api.marketJSONRequest(method, caixunOpenAPIBase+path, payload, headers)
	if err != nil {
		return nil, err
	}
	if resp.ResultCode == nil {
		return nil, fmt.Errorf("营销平台响应缺少 resultCode")
	}
	resp.Code, resp.Msg, resp.Result = resp.ResultCode, resp.ResultMsg, resp.ResultData
	if fmt.Sprint(resp.ResultCode) == "200" {
		resp.Code = 0
	}
	return resp, nil
}

// newActivityUUID 生成 RFC 4122 v4 UUID 字符串。
func newActivityUUID() (string, error) {
	buf := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("生成 UUID 失败: %w", err)
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(buf)
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32]), nil
}
