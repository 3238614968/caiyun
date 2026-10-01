package repository

import (
	"caiyun/internal/models"
	"testing"
	"time"
)

func TestDailyExchangeIgnoresLegacyRecurrenceAndCalendar(t *testing.T) {
	weekday, day := 5, 31
	for _, cycle := range []string{"weekly", "monthly", "once"} {
		task := &models.ExchangeTask{
			RestockTimes: "10:00,16:00,23:51", RestockCycle: cycle,
			RestockWeekday: &weekday, RestockDayOfMonth: &day,
			CustomCron: "invalid legacy cron", CalendarPolicy: "workday",
			HolidayDates: "2026-10-01", WorkdayDates: "invalid",
			AttemptedCount: 20,
		}
		for i := 0; i < 7; i++ {
			now := time.Date(2026, 10, 1+i, 10, 0, 0, 0, time.UTC)
			for _, hour := range []int{10, 16} {
				slot := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
				if ok, reason := ShouldRunExchangeTaskAtWithCalendar(task, slot, func(time.Time) (bool, bool) {
					t.Fatal("daily exchange queried holiday table")
					return true, true
				}); !ok {
					t.Fatalf("cycle=%s slot=%s rejected: %s", cycle, slot, reason)
				}
			}
			if ok, _ := ShouldRunExchangeTaskAt(task, now.Add(time.Minute)); ok {
				t.Fatal("unselected time was executed")
			}
		}
	}
}

func TestDailyNextRunUsesNextSelectedSlotIncludingMidnight(t *testing.T) {
	task := &models.ExchangeTask{RestockTimes: "16:00,00:00,10:00", CustomCron: "30 9 * * 5", CalendarPolicy: "holiday", RestockCycle: "monthly"}
	for _, tc := range []struct{ from, want string }{
		{"2026-10-01T09:59:00Z", "2026-10-01T10:00:00Z"},
		{"2026-10-01T10:00:01Z", "2026-10-01T16:00:00Z"},
		{"2026-10-01T16:01:00Z", "2026-10-02T00:00:00Z"},
	} {
		from, _ := time.Parse(time.RFC3339, tc.from)
		want, _ := time.Parse(time.RFC3339, tc.want)
		next := CalculateExchangeTaskNextRun(task, from)
		if next == nil || !next.Equal(want) {
			t.Fatalf("from=%s got=%v want=%s", from, next, want)
		}
	}
}
