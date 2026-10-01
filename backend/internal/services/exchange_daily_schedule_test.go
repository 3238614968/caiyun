package services

import (
	"caiyun/internal/models"
	"testing"
)

func TestTaskCreationKeepsDailySlotsWhenLegacyCronIsPresent(t *testing.T) {
	day := 99
	options := ExchangeTaskCreateOptions{
		TaskType: "long_term", RestockTimes: "10:00:00,16:00:00",
		CustomCron: "30 9 * * 5", RestockCycle: "weekly", RestockWeekday: &day,
		RestockDayOfMonth: &day, CalendarPolicy: "holiday", HolidayDates: "invalid", WorkdayDates: "invalid",
	}.withDefaults()
	if options.RestockTimes != "10:00:00,16:00:00" || options.CustomCron != "" || options.RestockCycle != "daily" || options.CalendarPolicy != "all" || options.RestockWeekday != nil || options.RestockDayOfMonth != nil || options.HolidayDates != "" || options.WorkdayDates != "" {
		t.Fatalf("selected slots overridden or daily defaults not applied: %+v", options)
	}
}

func TestLongTermExchangeContinuesAfterSoldOutAndExhaustedSlotRetries(t *testing.T) {
	task := &models.ExchangeTask{TaskType: "long_term", MaxAttempts: 1, AttemptedCount: 20, MaxRetries: 3}
	for _, reason := range []string{"奖品单日已耗尽", "奖品已兑完", "库存不足", "活动异常，请稍后重试！Error Code：GK", "云朵不足"} {
		if got := exchangeTaskFinalStatus(task, false, reason, 3); got != models.ExchangeTaskPending {
			t.Fatalf("long-term task stopped after %q: %s", reason, got)
		}
	}
	if got := exchangeTaskFinalStatus(task, true, "兑换成功", 0); got != models.ExchangeTaskPending {
		t.Fatal("long-term task stopped after success")
	}
	if got := exchangeTaskFinalStatus(task, false, "兑换结果待确认", 0); got != models.ExchangeTaskFailed {
		t.Fatal("uncertain acceptance was scheduled again")
	}
	task.TaskType = "fixed"
	if got := exchangeTaskFinalStatus(task, false, "云朵不足", 0); got != models.ExchangeTaskFailed {
		t.Fatal("single-run task repeated after a failed attempt")
	}
	if got := exchangeTaskFinalStatus(task, true, "兑换成功", 0); got != models.ExchangeTaskCompleted {
		t.Fatal("single-run task repeated after success")
	}
}
