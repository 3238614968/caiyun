package api

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// App 活动容器（hcyview / cloudCircle / cloudItem / yunClound）共用一把
// 1024-bit RSA 公钥，用于「公钥加密时间戳」的签名套路：
//
//	publicKeyEncrypt(e) -> JSEncrypt(publicKey).encrypt(JSON.stringify(e))
//	getEncryptTime()    -> opRequest({op:"currentTimeMillis"})，失败回落本地时间
//
// 加密方式为标准 PKCS#1 v1.5。凡是要提交「加密时间戳 + 公钥加密」的 H5 请求
// （接受邀请、领奖校验等）都走这里。
const appActivityPublicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDEdVKnXpmib/xkN/SYguTHTTd4
f1N3K8L/QmcWLKtyrdoFwENaaAZC1v471+ge9y3cAgsSZJNbW9LmPD/7W0KZ3K1H
XLS5PBMAGFW/CybJ8nE8+xCH6ypOhFMq504q9mDujhtOI54XvDC1BZnDvA5J1Opx
eJuOtRAQar/7BgU1nwIDAQAB
-----END PUBLIC KEY-----`

// AppActivityPublicKey 解析内置的 App 活动 RSA 公钥。
func AppActivityPublicKey() (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(appActivityPublicKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("解析 App 活动公钥失败：PEM 为空")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析 App 活动公钥失败: %w", err)
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("App 活动公钥类型不是 RSA")
	}
	return publicKey, nil
}

// EncryptActivityPayload 把对象序列化后用活动公钥加密，返回 base64 密文。
func EncryptActivityPayload(payload interface{}) (string, error) {
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("序列化加密载荷失败: %w", err)
	}
	return EncryptActivityString(string(plaintext))
}

// EncryptActivityString 用活动公钥加密字符串，返回 base64 密文。
func EncryptActivityString(plaintext string) (string, error) {
	publicKey, err := AppActivityPublicKey()
	if err != nil {
		return "", err
	}
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, []byte(plaintext))
	if err != nil {
		return "", fmt.Errorf("公钥加密失败: %w", err)
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// ActivityEncryptTimeMillis 获取用于签名的服务器毫秒时间戳，失败回落本地时间。
func (api *CaiyunAPI) ActivityEncryptTimeMillis() int64 {
	body, err := api.activityGetBody(
		"",
		MarketURL+"/portal/ajax/tools/opRequest.action?op=currentTimeMillis",
	)
	if err == nil {
		if value, ok := parseOpRequestTime(body); ok {
			return value
		}
	}
	return time.Now().UnixMilli()
}

// parseOpRequestTime 从 opRequest 回执里取出毫秒时间戳。
func parseOpRequestTime(body string) (int64, bool) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return 0, false
	}
	if value, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return value, true
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && len(envelope.Result) > 0 {
		candidate := strings.Trim(strings.TrimSpace(string(envelope.Result)), `"`)
		if value, err := strconv.ParseInt(candidate, 10, 64); err == nil {
			return value, true
		}
	}
	return 0, false
}
