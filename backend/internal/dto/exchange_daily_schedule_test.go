package dto

import (
	"caiyun/internal/models"
	"testing"
)

func TestExchangeTaskResponseDisplaysEffectiveDailySchedule(t *testing.T) {
	weekday := 5
	task := &models.ExchangeTask{
		RestockTimes: "10:00,16:00", RestockCycle: "weekly", RestockWeekday: &weekday,
		CustomCron: "30 9 * * 5", CalendarPolicy: "holiday", HolidayDates: "2026-10-01",
	}
	result := ToExchangeTaskResponse(task)
	if result.RestockTimes != task.RestockTimes || result.RestockCycle != "daily" || result.CalendarPolicy != "all" || result.CustomCron != "" || result.HolidayDates != "" || result.RestockWeekday != nil {
		t.Fatalf("API displayed ignored schedule options: %+v", result)
	}
	if task.RestockCycle != "weekly" || task.CustomCron == "" {
		t.Fatal("serialization mutated stored task")
	}
}
