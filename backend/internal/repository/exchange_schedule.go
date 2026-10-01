package repository

import (
	"caiyun/internal/models"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ExchangeTaskScheduleSkip 描述某个任务在当前时间槽被调度层跳过的原因。
type ExchangeTaskScheduleSkip struct {
	TaskID         uint
	RestockCycle   string
	CalendarPolicy string
	Reason         string
}

// CalendarLookup 为历史调用保留的类型；每日抢兑调度不再调用节假日查询。
type CalendarLookup func(time.Time) (isHoliday bool, found bool)

// ShouldRunExchangeTaskAt 判断任务在指定分钟是否应该执行，并返回跳过原因。
func ShouldRunExchangeTaskAt(task *models.ExchangeTask, now time.Time) (bool, string) {
	return ShouldRunExchangeTaskAtWithCalendar(task, now, nil)
}

// ShouldRunExchangeTaskAtWithCalendar 保留旧调用签名；所有任务只按每天的时间点判断。
// 历史 Cron、周期、节假日及调休字段不再限制抢兑，也不再查询节假日表。
func ShouldRunExchangeTaskAtWithCalendar(task *models.ExchangeTask, now time.Time, _ CalendarLookup) (bool, string) {
	if task == nil {
		return false, "任务为空"
	}
	return matchExchangeTaskTime(task, now)
}

// CalculateExchangeTaskNextRun 计算下一次每日时间点，用于 API 预览。
func CalculateExchangeTaskNextRun(task *models.ExchangeTask, from time.Time) *time.Time {
	return CalculateExchangeTaskNextRunWithCalendar(task, from, nil)
}

// CalculateExchangeTaskNextRunWithCalendar 兼容历史调用，忽略日历限制。
func CalculateExchangeTaskNextRunWithCalendar(task *models.ExchangeTask, from time.Time, _ CalendarLookup) *time.Time {
	times := exchangeTaskCandidateTimes(task)
	if len(times) == 0 {
		return nil
	}
	baseDate := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, from.Location())
	for dayOffset := 0; dayOffset <= 1; dayOffset++ {
		date := baseDate.AddDate(0, 0, dayOffset)
		for _, slot := range times {
			candidate := time.Date(date.Year(), date.Month(), date.Day(), slot.hour, slot.minute, 0, 0, from.Location())
			if !candidate.Before(from) {
				return &candidate
			}
		}
	}
	return nil
}

func matchExchangeTaskTime(task *models.ExchangeTask, now time.Time) (bool, string) {
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
	if len(values) <= 1 && strings.TrimSpace(task.ScheduledExchangeTime) != "" {
		values = []string{task.ScheduledExchangeTime}
	}
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
