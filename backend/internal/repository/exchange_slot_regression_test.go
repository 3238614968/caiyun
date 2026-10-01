package repository

import (
	"caiyun/internal/models"
	"testing"
	"time"
)

func TestWarmupDecisionUsesRequestedMinuteAndIgnoresUnrelatedTasks(t *testing.T) {
	warmup := time.Date(2026, 9, 30, 17, 50, 30, 0, time.FixedZone("CST", 8*3600))
	slot := time.Date(warmup.Year(), warmup.Month(), warmup.Day(), 17, 51, 0, 0, warmup.Location())
	task := &models.ExchangeTask{RestockTimes: "10:00,17:51", CalendarPolicy: "all"}
	if ok, _ := ShouldRunExchangeTaskAt(task, warmup); ok {
		t.Fatal("fixture must reproduce early-minute mismatch")
	}
	if ok, reason := exchangeTaskSlotDecision(task, slot, nil); !ok {
		t.Fatalf("warmup rejected target slot: %s", reason)
	}
	unrelated := &models.ExchangeTask{RestockTimes: "10:00,17:50"}
	if ok, reason := exchangeTaskSlotDecision(unrelated, slot, nil); ok || reason != "" {
		t.Fatalf("unrelated slot was recorded as a skip: %v %q", ok, reason)
	}
	task.CalendarPolicy = "workday"
	if ok, reason := exchangeTaskSlotDecision(task, slot, func(time.Time) (bool, bool) {
		t.Fatal("daily scheduling queried the holiday database")
		return true, true
	}); !ok || reason != "" {
		t.Fatal("obsolete calendar settings restricted daily exchange")
	}
}

func TestSpecifiedTimeOverridesSingleStaleReservationPreset(t *testing.T) {
	day := time.Date(2026, 9, 30, 17, 51, 0, 0, time.FixedZone("CST", 8*3600))
	task := &models.ExchangeTask{ScheduledExchangeTime: "17:51:00", RestockTimes: "10:30:00"}
	if ok, reason := ShouldRunExchangeTaskAt(task, day); !ok {
		t.Fatalf("old reservation preset overrode visible specified time: %s", reason)
	}
	if ok, _ := ShouldRunExchangeTaskAt(task, day.Add(-7*time.Hour-21*time.Minute)); ok {
		t.Fatal("stale 10:30 preset still executed")
	}
	task.RestockTimes = "10:30:00,16:00:00"
	if ok, _ := ShouldRunExchangeTaskAt(task, day); ok {
		t.Fatal("an explicitly configured multi-slot schedule must still take priority")
	}
}

func TestAllDaysPolicyDoesNotReadHolidayDatabase(t *testing.T) {
	if ok, _ := ShouldRunExchangeTaskAtWithCalendar(&models.ExchangeTask{ScheduledExchangeTime: "17:51"}, time.Date(2026, 9, 30, 17, 51, 0, 0, time.UTC), func(time.Time) (bool, bool) {
		t.Fatal("all-day policy performed an unnecessary holiday query")
		return false, false
	}); !ok {
		t.Fatal("all-day schedule did not match")
	}
}
