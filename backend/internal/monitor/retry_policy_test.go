package monitor

import (
	"errors"
	"testing"
	"time"
)

type completedBatchFailure struct{}

func (completedBatchFailure) Error() string   { return "partial batch failure" }
func (completedBatchFailure) Retryable() bool { return false }

func TestTerminalBatchDoesNotReplayCompletedMutations(t *testing.T) {
	monitor := NewTaskMonitor(Config{MaxHistory: 10})
	defer monitor.Stop()
	rm := NewRetryManager(monitor, 3, time.Millisecond)
	calls := 0
	err := rm.ExecuteWithRetry(1, "all_tasks", func() error { calls++; return completedBatchFailure{} }, nil)
	if err == nil || calls != 1 {
		t.Fatalf("completed mutations replayed: calls=%d err=%v", calls, err)
	}
	stats := monitor.GetStats()
	if stats["failed"] != int32(1) {
		t.Fatal("batch failure was reported as success")
	}
	if isNonRetryableTaskError(errors.New("temporary network failure")) {
		t.Fatal("network retry disabled")
	}
}
