package services

import (
	"caiyun/internal/constants"
	"testing"
	"time"
)

func TestScheduledPrepareSlotUsesNextMinute(t *testing.T) {
	now := time.Date(2026, 3, 17, 9, 59, 60-constants.ExchangePreInitSeconds, 0, time.Local)

	hour, minute, ok := scheduledPrepareSlot(now)
	if !ok {
		t.Fatal("expected prepare slot to be scheduled")
	}
	if hour != 10 || minute != 0 {
		t.Fatalf("expected 10:00, got %02d:%02d", hour, minute)
	}
}

func TestScheduledPrepareSlotSupportsCustomMinute(t *testing.T) {
	now := time.Date(2026, 3, 17, 10, 29, 60-constants.ExchangePreInitSeconds, 0, time.Local)

	hour, minute, ok := scheduledPrepareSlot(now)
	if !ok {
		t.Fatal("expected prepare slot to be scheduled")
	}
	if hour != 10 || minute != 30 {
		t.Fatalf("expected 10:30, got %02d:%02d", hour, minute)
	}
}

func TestScheduledExecuteSlotTriggersEveryMinuteBoundary(t *testing.T) {
	now := time.Date(2026, 3, 17, 10, 30, 0, 0, time.Local)

	hour, minute, ok := scheduledExecuteSlot(now)
	if !ok {
		t.Fatal("expected execute slot to be scheduled")
	}
	if hour != 10 || minute != 30 {
		t.Fatalf("expected 10:30, got %02d:%02d", hour, minute)
	}
}
