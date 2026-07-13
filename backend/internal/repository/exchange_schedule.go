package repository

import (
	"caiyun/internal/models"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// ExchangeTaskScheduleSkip 描述某个任务在当前时间槽被调度层跳过的原因。
type ExchangeTaskScheduleSkip struct {
	TaskID         uint
	RestockCycle   string
	CalendarPolicy string
	Reason         string
}

var exchangeCronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// CalendarLookup 从真实节假日表查询指定日期。返回 found=false 时调度层会回退周末判断。
type CalendarLookup func(time.Time) (isHoliday bool, found bool)

// ShouldRunExchangeTaskAt 判断任务在指定分钟是否应该执行，并返回跳过原因。
func ShouldRunExchangeTaskAt(task *models.ExchangeTask, now time.Time) (bool, string) {
	return ShouldRunExchangeTaskAtWithCalendar(task, now, nil)
}

// ShouldRunExchangeTaskAtWithCalendar 使用真实节假日表判断任务在指定分钟是否应该执行。
func ShouldRunExchangeTaskAtWithCalendar(task *models.ExchangeTask, now time.Time, lookup CalendarLookup) (bool, string) {
	if task == nil {
		return false, "任务为空"
	}
	if ok, reason := matchExchangeCalendarPolicy(task, now, lookup); !ok {
		return false, reason
	}
	if ok, reason := matchExchangeRestockCycle(task, now); !ok {
		return false, reason
	}
	if ok, reason := matchExchangeTaskTime(task, now); !ok {
		return false, reason
	}
	return true, ""
}

// CalculateExchangeTaskNextRun 计算任务下一次预计触发时间，用于 API 预览。
func CalculateExchangeTaskNextRun(task *models.ExchangeTask, from time.Time) *time.Time {
	return CalculateExchangeTaskNextRunWithCalendar(task, from, nil)
}

// CalculateExchangeTaskNextRunWithCalendar 使用真实节假日表计算下一次预计触发时间。
func CalculateExchangeTaskNextRunWithCalendar(task *models.ExchangeTask, from time.Time, lookup CalendarLookup) *time.Time {
	if task == nil {
		return nil
	}
	from = from.Truncate(time.Minute)
	if cronExpr := strings.TrimSpace(task.CustomCron); cronExpr != "" {
		schedule, err := exchangeCronParser.Parse(cronExpr)
		if err != nil {
			return nil
		}
		cursor := from.Add(-time.Second)
		for i := 0; i < 366*24*60; i++ {
			next := schedule.Next(cursor)
			if next.IsZero() || next.After(from.AddDate(1, 0, 0)) {
				return nil
			}
			if ok, _ := matchExchangeCalendarPolicy(task, next, lookup); ok {
				if ok, _ := matchExchangeRestockCycle(task, next); ok {
					return &next
				}
			}
			cursor = next
		}
		return nil
	}

	times := exchangeTaskCandidateTimes(task)
	if len(times) == 0 {
		return nil
	}

	baseDate := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	for dayOffset := 0; dayOffset <= 366; dayOffset++ {
		date := baseDate.AddDate(0, 0, dayOffset)
		for _, slot := range times {
			candidate := time.Date(date.Year(), date.Month(), date.Day(), slot.hour, slot.minute, 0, 0, from.Location())
			if candidate.Before(from) {
				continue
			}
			if ok, _ := matchExchangeCalendarPolicy(task, candidate, lookup); !ok {
				continue
			}
			if ok, _ := matchExchangeRestockCycle(task, candidate); !ok {
				continue
			}
			return &candidate
		}
	}
	return nil
}

func matchExchangeTaskTime(task *models.ExchangeTask, now time.Time) (bool, string) {
	if cronExpr := strings.TrimSpace(task.CustomCron); cronExpr != "" {
		schedule, err := exchangeCronParser.Parse(cronExpr)
		if err != nil {
			return false, "自定义 cron 格式错误"
		}
		slot := now.Truncate(time.Minute)
		next := schedule.Next(slot.Add(-time.Minute))
		if !next.Before(slot) && next.Before(slot.Add(time.Minute)) {
			return true, ""
		}
		return false, fmt.Sprintf("自定义 cron 未匹配当前时间槽 %s", slot.Format("15:04"))
	}

	times := exchangeTaskCandidateTimes(task)
	if len(times) == 0 {
		return false, "未配置抢兑时间"
	}
	for _, slot := range times {
		if slot.hour == now.Hour() && slot.minute == now.Minute() {
			return true, ""
		}
	}
	return false, fmt.Sprintf("当前时间 %s 不在任务抢兑时间点内", now.Format("15:04"))
}

type exchangeSlotTime struct{ hour, minute int }

func exchangeTaskCandidateTimes(task *models.ExchangeTask) []exchangeSlotTime {
	if task == nil {
		return nil
	}
	values := splitCSVLike(task.RestockTimes)
	if len(values) == 0 {
		if strings.TrimSpace(task.ScheduledExchangeTime) != "" {
			values = append(values, task.ScheduledExchangeTime)
		} else {
			values = append(values, task.ExchangeAccount.ExchangeTime1, task.ExchangeAccount.ExchangeTime2)
		}
	}
	seen := map[string]struct{}{}
	times := make([]exchangeSlotTime, 0, len(values))
	for _, value := range values {
		parsed, ok := parseExchangeHHMM(value)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%02d:%02d", parsed.hour, parsed.minute)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		times = append(times, parsed)
	}
	sort.Slice(times, func(i, j int) bool {
		if times[i].hour == times[j].hour {
			return times[i].minute < times[j].minute
		}
		return times[i].hour < times[j].hour
	})
	return times
}

func parseExchangeHHMM(value string) (exchangeSlotTime, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return exchangeSlotTime{}, false
	}
	for _, layout := range []string{"15:04:05", "15:04"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return exchangeSlotTime{hour: parsed.Hour(), minute: parsed.Minute()}, true
		}
	}
	return exchangeSlotTime{}, false
}

func matchExchangeRestockCycle(task *models.ExchangeTask, now time.Time) (bool, string) {
	cycle := strings.ToLower(strings.TrimSpace(task.RestockCycle))
	if cycle == "" {
		cycle = "daily"
	}
	switch cycle {
	case "daily":
		return true, ""
	case "weekly":
		if task.RestockWeekday == nil {
			return true, ""
		}
		if *task.RestockWeekday == int(now.Weekday()) {
			return true, ""
		}
		return false, fmt.Sprintf("补货周期为每周 %d，当前星期 %d 不匹配", *task.RestockWeekday, int(now.Weekday()))
	case "monthly":
		if task.RestockDayOfMonth == nil {
			return true, ""
		}
		if *task.RestockDayOfMonth == now.Day() {
			return true, ""
		}
		return false, fmt.Sprintf("补货周期为每月 %d 日，当前日期 %d 不匹配", *task.RestockDayOfMonth, now.Day())
	case "once":
		if task.AttemptedCount <= 0 && task.LastAttemptAt == nil {
			return true, ""
		}
		return false, "仅一次任务已尝试过，等待人工处理"
	default:
		return true, ""
	}
}

func matchExchangeCalendarPolicy(task *models.ExchangeTask, now time.Time, lookup CalendarLookup) (bool, string) {
	policy := strings.ToLower(strings.TrimSpace(task.CalendarPolicy))
	if policy == "" {
		policy = "all"
	}
	isHoliday := isExchangeHoliday(task, now, lookup)
	switch policy {
	case "all":
		return true, ""
	case "workday":
		if isHoliday {
			return false, "日历策略为工作日，当前为节假日/周末"
		}
		return true, ""
	case "holiday":
		if !isHoliday {
			return false, "日历策略为节假日，当前为工作日"
		}
		return true, ""
	default:
		return true, ""
	}
}

func isExchangeHoliday(task *models.ExchangeTask, now time.Time, lookup CalendarLookup) bool {
	date := now.Format("2006-01-02")
	if stringSetContains(task.WorkdayDates, date) {
		return false
	}
	if stringSetContains(task.HolidayDates, date) {
		return true
	}
	if lookup != nil {
		if holiday, found := lookup(now); found {
			return holiday
		}
	}
	weekday := now.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}

func stringSetContains(raw, needle string) bool {
	for _, item := range splitCSVLike(raw) {
		if item == needle {
			return true
		}
	}
	return false
}

func splitCSVLike(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r' || r == '\t'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}
