package services

import (
	"testing"
	"time"

	"caiyun/internal/models"
)

func TestScheduledSlots(t *testing.T) {
	executeAt := time.Date(2026, 6, 6, 10, 0, 0, 0, time.Local)
	hour, minute, ok := scheduledExecuteSlot(executeAt)
	if !ok || hour != 10 || minute != 0 {
		t.Fatalf("scheduledExecuteSlot() = %d:%d %t, want 10:0 true", hour, minute, ok)
	}

	notExactSecond := time.Date(2026, 6, 6, 10, 0, 1, 0, time.Local)
	if _, _, ok := scheduledExecuteSlot(notExactSecond); ok {
		t.Fatal("scheduledExecuteSlot should only trigger at second 0")
	}
}

func TestMergeExchangeTasksDeduplicates(t *testing.T) {
	merged := mergeExchangeTasks(
		[]*models.ExchangeTask{{ID: 1}, {ID: 2}},
		[]*models.ExchangeTask{{ID: 2}, {ID: 3}, nil},
	)
	if len(merged) != 3 {
		t.Fatalf("len(merged) = %d, want 3", len(merged))
	}
	for i, wantID := range []uint{1, 2, 3} {
		if merged[i].ID != wantID {
			t.Fatalf("merged[%d].ID = %d, want %d", i, merged[i].ID, wantID)
		}
	}
}
