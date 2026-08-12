package bootstrap

import (
	"testing"
	"time"
)

func TestLoadCoreConfigUsesOneStructuredBoundary(t *testing.T) {
	t.Setenv("DB_HOST", "mysql.test")
	t.Setenv("DB_PORT", "3307")
	t.Setenv("DB_USER", "app")
	t.Setenv("DB_NAME", "caiyun_test")
	t.Setenv("DB_MAX_IDLE_CONNS", "7")
	t.Setenv("DB_MAX_OPEN_CONNS", "17")
	t.Setenv("DB_CONN_MAX_LIFETIME", "2m")
	t.Setenv("DB_CONN_MAX_IDLE_TIME", "3m")
	t.Setenv("REDIS_HOST", "redis.test")
	t.Setenv("REDIS_PORT", "6380")
	t.Setenv("REDIS_DB", "4")
	t.Setenv("CLOCK_SKEW_MAX", "45s")
	t.Setenv("TASK_CONFIG_SYNC_ON_STARTUP", "true")

	config, err := LoadCoreConfig()
	if err != nil {
		t.Fatalf("LoadCoreConfig() error = %v", err)
	}
	if config.Database.Host != "mysql.test" || config.Database.Port != "3307" || config.Database.MaxOpenConns != 17 {
		t.Fatalf("database config = %#v", config.Database)
	}
	if config.Redis.Host != "redis.test" || config.Redis.Port != "6380" || config.Redis.DB != 4 {
		t.Fatalf("redis config = %#v", config.Redis)
	}
	if config.ClockSkewMax != 45*time.Second {
		t.Fatalf("ClockSkewMax = %s", config.ClockSkewMax)
	}
	if !config.SyncTaskDefinitions {
		t.Fatal("SyncTaskDefinitions = false, want true")
	}
}

func TestLoadCoreConfigTaskDefinitionSyncDefaultsOff(t *testing.T) {
	t.Setenv("TASK_CONFIG_SYNC_ON_STARTUP", "")
	config, err := LoadCoreConfig()
	if err != nil {
		t.Fatalf("LoadCoreConfig() error = %v", err)
	}
	if config.SyncTaskDefinitions {
		t.Fatal("SyncTaskDefinitions = true, want release-safe default false")
	}
}

func TestLoadCoreConfigRejectsInvalidPool(t *testing.T) {
	t.Setenv("DB_MAX_OPEN_CONNS", "0")
	_, err := LoadCoreConfig()
	if err == nil {
		t.Fatal("LoadCoreConfig() error = nil, want invalid pool error")
	}
}
