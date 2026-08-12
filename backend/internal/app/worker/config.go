package worker

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/observability"
	"caiyun/internal/version"
)

// Config is the complete Worker-process configuration. Core infrastructure
// settings remain in bootstrap.CoreConfig so they can be shared with API.
type Config struct {
	TaskConcurrency int
	Retry           RetryConfig
	TaskSchedule    string
	Realtime        RealtimeConfig
	Monitoring      MonitoringConfig
	Tracing         observability.TraceConfig
}

type RetryConfig struct {
	MaxAttempts int
	Delay       time.Duration
}

type RealtimeConfig struct {
	EventChannel string
	InstanceID   string
}

type MonitoringConfig struct {
	Host           string
	Port           string
	AllowPlaintext bool
	Token          string
}

// LoadConfig resolves every Worker-process setting once and validates values
// that affect scheduling, retry behavior, realtime delivery, and monitoring.
func LoadConfig() (Config, error) {
	concurrency, err := positiveIntEnv("TASK_CONCURRENCY", 10)
	if err != nil {
		return Config{}, err
	}
	maxAttempts, err := positiveIntEnv("WORKER_RETRY_MAX_ATTEMPTS", 3)
	if err != nil {
		return Config{}, err
	}
	config := Config{
		TaskConcurrency: concurrency,
		Retry: RetryConfig{
			MaxAttempts: maxAttempts,
			Delay:       bootstrap.GetDurationEnv("WORKER_RETRY_DELAY", 5*time.Second),
		},
		TaskSchedule: strings.TrimSpace(bootstrap.GetEnv("TASK_SCHEDULE", "0 8 * * *")),
		Realtime: RealtimeConfig{
			EventChannel: strings.TrimSpace(bootstrap.GetEnv("WS_EVENT_CHANNEL", "caiyun:ws:events")),
			InstanceID:   strings.TrimSpace(bootstrap.GetEnv("INSTANCE_ID", "worker")),
		},
		Monitoring: MonitoringConfig{
			Host:           strings.TrimSpace(bootstrap.GetEnv("WORKER_MONITOR_HOST", "127.0.0.1")),
			Port:           strings.TrimSpace(bootstrap.GetEnv("WORKER_MONITOR_PORT", "8081")),
			AllowPlaintext: bootstrap.GetBoolEnv("WORKER_MONITOR_ALLOW_PLAINTEXT", false),
			Token:          bootstrap.GetEnv("WORKER_MONITOR_TOKEN", ""),
		},
	}
	if config.Retry.Delay <= 0 {
		return Config{}, fmt.Errorf("WORKER_RETRY_DELAY 必须为正数")
	}
	if config.TaskSchedule == "" {
		return Config{}, fmt.Errorf("TASK_SCHEDULE 不能为空")
	}
	if config.Realtime.EventChannel == "" || config.Realtime.InstanceID == "" {
		return Config{}, fmt.Errorf("实时事件通道和实例标识均不能为空")
	}
	if config.Monitoring.Host == "" || config.Monitoring.Port == "" {
		return Config{}, fmt.Errorf("Worker 监控地址不能为空")
	}
	tracing, err := observability.LoadTraceConfig("caiyun-worker", version.Get().Version)
	if err != nil {
		return Config{}, fmt.Errorf("OTel 配置无效: %w", err)
	}
	config.Tracing = tracing
	return config, nil
}

func positiveIntEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(bootstrap.GetEnv(key, ""))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s 必须为正整数", key)
	}
	return value, nil
}
