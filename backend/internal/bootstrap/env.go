package bootstrap

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// LoadEnvFile loads .env when present; missing files are expected in containerized deployments.
func LoadEnvFile() {
	if err := godotenv.Load(); err != nil {
		log.Println("未找到 .env 文件，使用环境变量和默认配置")
	}
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

func GetSecretEnv(key, insecureDefault string) string {
	value, exists := os.LookupEnv(key)
	if !exists || value == "" {
		if GetBoolEnv("ALLOW_INSECURE_DEFAULTS", false) {
			log.Printf("警告：%s 使用不安全默认值，仅允许本地调试", key)
			return insecureDefault
		}
		log.Fatalf("缺少必需环境变量 %s；如仅本地调试可设置 ALLOW_INSECURE_DEFAULTS=true", key)
	}
	if value == insecureDefault || len(value) < 16 {
		if GetBoolEnv("ALLOW_INSECURE_DEFAULTS", false) {
			log.Printf("警告：%s 使用弱值，仅允许本地调试", key)
			return value
		}
		log.Fatalf("%s 使用弱值或默认值，请更换为强随机值", key)
	}
	return value
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
