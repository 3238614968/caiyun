package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MD5 计算 MD5 哈希
func MD5(data string) string {
	h := md5.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// SHA256 计算 SHA256 哈希
func SHA256(data string) string {
	h := sha256.New()
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// RandomString 生成随机字符串
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			log.Printf("[WARN] RandomString rand.Int 失败: %v", err)
			return ""
		}
		b[i] = charset[n.Int64()]
	}
	return string(b)
}

// RandomHex 生成随机十六进制字符串
func RandomHex(length int) string {
	b := make([]byte, length/2)
	if _, err := rand.Read(b); err != nil {
		// 系统熵不足时记录并返回空串，调用方应检查长度。
		log.Printf("[WARN] RandomHex rand.Read 失败: %v", err)
		return ""
	}
	return hex.EncodeToString(b)
}

// GenerateUUID 生成 UUID
func GenerateUUID() string {
	return uuid.New().String()
}

// FormatTime 格式化时间
func FormatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// CurrentTimestamp 获取当前时间戳（毫秒）
func CurrentTimestamp() int64 {
	return time.Now().UnixMilli()
}

// Base64Encode Base64 编码
func Base64Encode(data string) string {
	return base64.StdEncoding.EncodeToString([]byte(data))
}

// Base64Decode Base64 解码
func Base64Decode(data string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// ParseToken 解析 Token
func ParseToken(token string) (int64, error) {
	// Token 格式分析：根据实际 Token 格式解析过期时间
	// 这里需要根据实际的 Token 格式进行调整
	// 假设 Token 是 JSON 格式或者包含过期时间字段

	// 临时实现：从 Token 中提取信息
	// 实际实现需要根据具体 Token 格式解析
	if token == "" {
		return 0, fmt.Errorf("token 为空")
	}

	// 默认返回当前时间 + 30 天
	return time.Now().Add(30 * 24 * time.Hour).UnixMilli(), nil
}

// defaultAiUserIDKey 是与移动云盘 AI 云朵接口约定的 AES-128 密钥。
// 当未配置 CAIYUN_AI_USERID_KEY 时回退使用，并打印警告日志。
// 注意：该密钥由服务端协议定义，非项目自有密钥。
const defaultAiUserIDKey = "xuL97!x7GGxG%8V4"

// aiUserIDKey 返回用于加密 AI 用户 ID 的 AES-128 密钥。
// 优先读取环境变量 CAIYUN_AI_USERID_KEY；未配置时回退到协议默认密钥并告警。
func aiUserIDKey() ([]byte, error) {
	if envKey := strings.TrimSpace(os.Getenv("CAIYUN_AI_USERID_KEY")); len(envKey) == 16 {
		return []byte(envKey), nil
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GIN_MODE")), "release") &&
		!strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_INSECURE_DEFAULTS")), "true") {
		return nil, fmt.Errorf("CAIYUN_AI_USERID_KEY 未配置或长度非 16")
	}
	log.Printf("[WARN] CAIYUN_AI_USERID_KEY 未配置或长度非 16，回退到协议默认密钥")
	return []byte(defaultAiUserIDKey), nil
}

// EncryptAiUserId 加密用户ID（用于 AI 云朵）
// 对应 mjs ZT 函数：AES-128-CBC 加密后双重 Base64 编码
func EncryptAiUserId(userID string) (string, error) {
	key, err := aiUserIDKey()
	if err != nil {
		return "", err
	}

	// 生成 16 字节随机 IV
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("生成 AI 用户 ID 加密 IV 失败: %w", err)
	}

	// AES-128-CBC 加密
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("初始化 AI 用户 ID 加密器失败: %w", err)
	}

	// PKCS7 填充
	plaintext := []byte(userID)
	blockSize := block.BlockSize()
	padding := blockSize - len(plaintext)%blockSize
	padtext := make([]byte, len(plaintext)+padding)
	copy(padtext, plaintext)
	for i := len(plaintext); i < len(padtext); i++ {
		padtext[i] = byte(padding)
	}

	ciphertext := make([]byte, len(padtext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padtext)

	// IV + 密文
	combined := append(iv, ciphertext...)

	// 双重 Base64 编码
	firstBase64 := base64.StdEncoding.EncodeToString(combined)
	return base64.StdEncoding.EncodeToString([]byte(firstBase64)), nil
}

// ParseAuthString 解析 Auth 字符串
// 格式: Basic base64(platform:phone:token)
func ParseAuthString(auth string) (platform, phone, token string, err error) {
	if !strings.HasPrefix(auth, "Basic ") {
		err = fmt.Errorf("无效的 Auth 格式")
		return
	}

	encoded := strings.TrimPrefix(auth, "Basic ")
	decoded, err := Base64Decode(encoded)
	if err != nil {
		return "", "", "", fmt.Errorf("解码 Auth 失败: %w", err)
	}

	parts := strings.Split(decoded, ":")
	if len(parts) < 3 {
		err = fmt.Errorf("Auth 格式错误，应为 platform:phone:token")
		return
	}

	platform = parts[0]
	phone = parts[1]
	token = strings.Join(parts[2:], ":")

	return
}

// GenerateAuthString 生成 Auth 字符串
func GenerateAuthString(platform, phone, token string) string {
	data := fmt.Sprintf("%s:%s:%s", platform, phone, token)
	return "Basic " + Base64Encode(data)
}

// RandomInt 生成指定范围内的随机整数 [min, max]
func RandomInt(min, max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max-min+1)))
	return min + int(n.Int64())
}

// Sleep 休眠
func Sleep(duration int) {
	time.Sleep(time.Duration(duration) * time.Millisecond)
}

// ExtractXMLTag 从 XML 字符串中提取指定标签的内容
// 例如: ExtractXMLTag("<root><token>abc</token></root>", "token") 返回 "abc"
func ExtractXMLTag(xmlStr, tagName string) string {
	startTag := fmt.Sprintf("<%s>", tagName)
	endTag := fmt.Sprintf("</%s>", tagName)

	startIdx := strings.Index(xmlStr, startTag)
	if startIdx == -1 {
		return ""
	}

	startIdx += len(startTag)
	endIdx := strings.Index(xmlStr[startIdx:], endTag)
	if endIdx == -1 {
		return ""
	}

	return xmlStr[startIdx : startIdx+endIdx]
}
