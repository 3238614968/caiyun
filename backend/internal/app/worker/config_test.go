package worker

import (
	"testing"
	"time"
)

func TestLoadConfigBuildsStructuredWorkerConfig(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "false")
	t.Setenv("TASK_CONCURRENCY", "12")
	t.Setenv("WORKER_RETRY_MAX_ATTEMPTS", "5")
	t.Setenv("WORKER_RETRY_DELAY", "7s")
	t.Setenv("TASK_SCHEDULE", "0 9 * * *")
	t.Setenv("WS_EVENT_CHANNEL", "test:events")
	t.Setenv("INSTANCE_ID", "worker-test")
	t.Setenv("WORKER_MONITOR_HOST", "127.0.0.1")
	t.Setenv("WORKER_MONITOR_PORT", "18081")

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if config.TaskConcurrency != 12 || config.Retry.MaxAttempts != 5 || config.Retry.Delay != 7*time.Second {
		t.Fatalf("worker config = %#v", config)
	}
	if config.Realtime.EventChannel != "test:events" || config.Monitoring.Port != "18081" {
		t.Fatalf("realtime/monitoring config = %#v", config)
	}
}

func TestLoadConfigRejectsInvalidConcurrency(t *testing.T) {
	t.Setenv("OTEL_ENABLED", "false")
	t.Setenv("TASK_CONCURRENCY", "zero")
	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want invalid concurrency error")
	}
}
