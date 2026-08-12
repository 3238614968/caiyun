package api

import (
	"fmt"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/observability"
	"caiyun/internal/services"
	"caiyun/internal/version"
)

// Config is the complete process-level API configuration. Infrastructure
// settings are owned separately by bootstrap.CoreConfig.
type Config struct {
	Server   ServerConfig
	JWT      JWTConfig
	SMTP     services.SMTPConfig
	Realtime RealtimeConfig
	Tracing  observability.TraceConfig
}

type ServerConfig struct {
	Port             string
	TrustedProxies   string
	RequestTimeout   time.Duration
	MaxMultipartSize int64
}

type JWTConfig struct {
	Algorithm      string
	AccessTTL      time.Duration
	RefreshTTL     time.Duration
	Secret         string
	PrivateKey     string
	PublicKey      string
	Issuer         string
	Audience       string
}

type RealtimeConfig struct {
	EventChannel string
	InstanceID   string
}

// LoadConfig resolves and validates every API-process setting once. Handlers
// and service assembly receive the resulting dependencies instead of reading
// mutable process environment variables themselves.
func LoadConfig() (Config, error) {
	config := Config{
		Server: ServerConfig{
			Port:             strings.TrimSpace(bootstrap.GetEnv("PORT", "8080")),
			TrustedProxies:   bootstrap.GetEnv("TRUSTED_PROXIES", ""),
			RequestTimeout:   bootstrap.GetDurationEnv("REQUEST_TIMEOUT", 30*time.Second),
			MaxMultipartSize: 8 << 20,
		},
		JWT: JWTConfig{
			Algorithm:  strings.ToUpper(strings.TrimSpace(bootstrap.GetEnv("JWT_ALGORITHM", "HS256"))),
			AccessTTL:  bootstrap.GetDurationEnv("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL: bootstrap.GetDurationEnv("JWT_REFRESH_TTL", 30*24*time.Hour),
			Issuer:     bootstrap.GetEnv("JWT_ISSUER", "caiyun-api"),
			Audience:   bootstrap.GetEnv("JWT_AUDIENCE", "caiyun-web"),
		},
		SMTP: services.SMTPConfig{
			Host:     bootstrap.GetEnv("SMTP_HOST", ""),
			Port:     bootstrap.GetEnv("SMTP_PORT", "587"),
			Username: bootstrap.GetEnv("SMTP_USERNAME", ""),
			Password: bootstrap.GetEnv("SMTP_PASSWORD", ""),
			From:     bootstrap.GetEnv("SMTP_FROM", ""),
			FromName: bootstrap.GetEnv("SMTP_FROM_NAME", "移动云盘"),
			UseTLS:   bootstrap.GetBoolEnv("SMTP_USE_TLS", false),
		},
		Realtime: RealtimeConfig{
			EventChannel: bootstrap.GetEnv("WS_EVENT_CHANNEL", "caiyun:ws:events"),
			InstanceID:   bootstrap.GetEnv("INSTANCE_ID", "api"),
		},
	}

	if config.Server.Port == "" {
		return Config{}, fmt.Errorf("PORT 不能为空")
	}
	if config.Server.RequestTimeout <= 0 {
		return Config{}, fmt.Errorf("REQUEST_TIMEOUT 必须为正数")
	}
	if config.JWT.AccessTTL <= 0 || config.JWT.AccessTTL > time.Hour {
		return Config{}, fmt.Errorf("JWT_ACCESS_TTL 必须在 0 到 1h 之间")
	}
	if config.JWT.RefreshTTL <= config.JWT.AccessTTL {
		return Config{}, fmt.Errorf("JWT_REFRESH_TTL 必须大于 JWT_ACCESS_TTL")
	}
	if config.Realtime.EventChannel == "" || config.Realtime.InstanceID == "" {
		return Config{}, fmt.Errorf("实时事件通道和实例标识均不能为空")
	}
	tracing, err := observability.LoadTraceConfig("caiyun-api", version.Get().Version)
	if err != nil {
		return Config{}, fmt.Errorf("OTel 配置无效: %w", err)
	}
	config.Tracing = tracing

	switch config.JWT.Algorithm {
	case "HS256", "":
		config.JWT.Algorithm = "HS256"
		config.JWT.Secret = bootstrap.GetSecretEnv("JWT_SECRET", "")
	case "RS256":
		config.JWT.PrivateKey = bootstrap.GetSecretEnv("JWT_PRIVATE_KEY", "")
		config.JWT.PublicKey = bootstrap.GetSecretEnv("JWT_PUBLIC_KEY", "")
	default:
		return Config{}, fmt.Errorf("JWT_ALGORITHM=%q 无效，仅支持 HS256 或 RS256", config.JWT.Algorithm)
	}
	return config, nil
}
