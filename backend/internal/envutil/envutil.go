package envutil

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// RawString preserves an explicitly configured empty value. Startup/bootstrap
// configuration uses it for credentials such as an optional Redis password.
func RawString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// String 读取字符串环境变量；空值时返回默认值。
func String(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// Bool 读取布尔环境变量；非法值时返回默认值。
func Bool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// Int 读取整型环境变量；非法值时返回默认值。
func Int(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

// Int64 读取 int64 环境变量；非法值时返回默认值。
func Int64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

// Duration 读取时间环境变量；支持 Go duration 与秒数写法。
func Duration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
		return duration
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
