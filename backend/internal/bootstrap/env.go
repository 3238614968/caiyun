package bootstrap

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// LoadEnvFile loads .env when present.
// It first checks the current working directory, then the executable directory.
// 宝塔/进程管理器有时不会把工作目录切到二进制所在目录，第二个查找路径可避免 .env 未加载导致启动失败。
func LoadEnvFile() {
	if err := godotenv.Load(); err == nil {
		return
	}

	exePath, err := os.Executable()
	if err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(exePath); resolveErr == nil {
			exePath = resolved
		}
		envPath := filepath.Join(filepath.Dir(exePath), ".env")
		if loadErr := godotenv.Load(envPath); loadErr == nil {
			log.Printf("已从程序目录加载环境配置: %s", envPath)
			return
		}
	}

	log.Println("未找到 .env 文件，使用环境变量和默认配置")
}

func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func GetBoolEnv(key string, defaultValue bool) bool {
	value := strings.TrimSpace(strings.ToLower(GetEnv(key, "")))
	if value == "" {
		return defaultValue
	}
	return value == "true" || value == "1" || value == "yes" || value == "on"
}

func GetIntEnv(key string, defaultValue int) int {
	value := strings.TrimSpace(GetEnv(key, ""))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func GetDurationEnv(key string, defaultValue time.Duration) time.Duration {
	value := strings.TrimSpace(GetEnv(key, ""))
	if value == "" {
		return defaultValue
	}
	if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
		return duration
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return defaultValue
	}
	return time.Duration(seconds) * time.Second
}

// minSecretLength 是 HS256 等对称签名密钥的最小安全长度（字节）。
// HMAC-SHA-256 安全基线要求密钥 ≥ 32 字节（256 位）。
const minSecretLength = 32

func GetSecretEnv(key, insecureDefault string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		if GetBoolEnv("ALLOW_INSECURE_DEFAULTS", false) {
			log.Printf("警告：%s 使用不安全默认值，仅允许本地调试", key)
			return insecureDefault
		}
		exitConfigErrorf("缺少必需环境变量 %s；如仅本地调试可设置 ALLOW_INSECURE_DEFAULTS=true", key)
	}
	if value == insecureDefault || isPlaceholderSecret(value) || len(value) < minSecretLength {
		if GetBoolEnv("ALLOW_INSECURE_DEFAULTS", false) {
			log.Printf("警告：%s 使用弱值，仅允许本地调试", key)
			return value
		}
		exitConfigErrorf("%s 使用弱值或默认值，请更换为至少 %d 字符的强随机值", key, minSecretLength)
	}
	return value
}

func exitConfigErrorf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log.Print(msg)
	_, _ = fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func isPlaceholderSecret(value string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	return strings.HasPrefix(normalized, "__GENERATE") ||
		strings.Contains(normalized, "YOUR_") ||
		strings.Contains(normalized, "REPLACE_")
}

// ToInt 将常见数值类型安全转换为 int。
func ToInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}
