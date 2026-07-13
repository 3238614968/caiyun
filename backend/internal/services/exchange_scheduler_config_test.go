package services

import (
	"testing"
	"time"
)

func TestExchangeTaskRunningTimeoutFromEnv(t *testing.T) {
	t.Setenv("EXCHANGE_TASK_RUNNING_TIMEOUT", "2m")
	if got := exchangeTaskRunningTimeoutFromEnv(); got != 2*time.Minute {
		t.Fatalf("duration timeout=%s, want 2m", got)
	}

	t.Setenv("EXCHANGE_TASK_RUNNING_TIMEOUT", "45")
	if got := exchangeTaskRunningTimeoutFromEnv(); got != 45*time.Second {
		t.Fatalf("seconds timeout=%s, want 45s", got)
	}

	t.Setenv("EXCHANGE_TASK_RUNNING_TIMEOUT", "bad")
	if got := exchangeTaskRunningTimeoutFromEnv(); got != defaultExchangeTaskRunningTimeout {
		t.Fatalf("invalid timeout=%s, want default %s", got, defaultExchangeTaskRunningTimeout)
	}
}
