package repository

import (
	"fmt"
	"time"

	"caiyun/internal/models"
)

// Scheduler selection is deliberately read-oriented; skip annotations use the
// narrow write method rather than persisting an entire task model.

// GetTasksByTimeAtWithSkips 返回当前时间槽可执行任务以及因周期/日历/时间策略跳过的任务摘要。
func (r *ExchangeTaskRepository) GetTasksByTimeAtWithSkips(hour, minute int, now time.Time) ([]*models.ExchangeTask, []ExchangeTaskScheduleSkip, error) {
	// Warm-up evaluates the requested slot, not the earlier wall-clock minute.
	slot := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	var tasks []*models.ExchangeTask
	timeStr := fmt.Sprintf("%02d:%02d:00", hour, minute)
	clockValues := []string{timeStr, timeStr[:5]}

	err := r.db.Joins("JOIN exchange_rules ON exchange_rules.id = exchange_tasks.exchange_rule_id").
		Joins("JOIN accounts ON accounts.id = exchange_rules.account_id").
		Where("exchange_tasks.status = ?", string(models.ExchangeTaskPending)).
		Where(`(
			(exchange_tasks.restock_times IS NOT NULL AND exchange_tasks.restock_times LIKE ?)
			OR (exchange_tasks.scheduled_exchange_time IS NOT NULL AND exchange_tasks.scheduled_exchange_time <> '' AND exchange_tasks.scheduled_exchange_time IN ?)
			OR ((exchange_tasks.scheduled_exchange_time IS NULL OR exchange_tasks.scheduled_exchange_time = '') AND (exchange_rules.exchange_time_1 IN ? OR exchange_rules.exchange_time_2 IN ?))
		)`, "%"+timeStr[:5]+"%", clockValues, clockValues, clockValues).
		Where("exchange_rules.is_active = ?", true).
		Where("accounts.is_active = ?", true).
		Where("accounts.auth <> ''").
		Where("exchange_rules.deleted_at IS NULL AND accounts.deleted_at IS NULL").
		Where("exchange_tasks.deleted_at IS NULL").
		Preload("ExchangeAccount").
		Preload("ExchangeAccount.Account").
		Preload("Product").
		Order("exchange_tasks.priority DESC, exchange_tasks.created_at ASC").
		Find(&tasks).Error
	if err != nil {
		return nil, nil, err
	}

	runnable := make([]*models.ExchangeTask, 0, len(tasks))
	skipped := make([]ExchangeTaskScheduleSkip, 0)
	for _, task := range tasks {
		ok, reason := exchangeTaskSlotDecision(task, slot, r.lookupCalendarHoliday)
		if !ok && reason == "" {
			continue
		}
		if ok {
			if task.SkipReason != "" {
				_ = r.UpdateSkipReason(task.ID, "")
				task.SkipReason = ""
			}
			runnable = append(runnable, task)
		} else {
			if task.SkipReason != reason {
				_ = r.UpdateSkipReason(task.ID, reason)
			}
			task.SkipReason = reason
			skipped = append(skipped, ExchangeTaskScheduleSkip{
				TaskID:         task.ID,
				RestockCycle:   task.RestockCycle,
				CalendarPolicy: task.CalendarPolicy,
				Reason:         reason,
			})
		}
	}
	return runnable, skipped, nil
}

func exchangeTaskSlotDecision(task *models.ExchangeTask, slot time.Time, lookup CalendarLookup) (bool, string) {
	if matches, _ := matchExchangeTaskTime(task, slot); !matches {
		return false, ""
	}
	return ShouldRunExchangeTaskAtWithCalendar(task, slot, lookup)
}
