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
	var tasks []*models.ExchangeTask
	timeStr := fmt.Sprintf("%02d:%02d:00", hour, minute)

	err := r.db.Joins("JOIN exchange_rules ON exchange_rules.id = exchange_tasks.exchange_rule_id").
		Joins("JOIN accounts ON accounts.id = exchange_rules.account_id").
		Where("exchange_tasks.status IN ?", []string{string(models.ExchangeTaskPending), string(models.ExchangeTaskRunning)}).
		Where(`(
			(exchange_tasks.custom_cron IS NOT NULL AND exchange_tasks.custom_cron <> '')
			OR (exchange_tasks.restock_times IS NOT NULL AND exchange_tasks.restock_times <> '')
			OR (exchange_tasks.scheduled_exchange_time IS NOT NULL AND exchange_tasks.scheduled_exchange_time <> '' AND exchange_tasks.scheduled_exchange_time = ?)
			OR ((exchange_tasks.scheduled_exchange_time IS NULL OR exchange_tasks.scheduled_exchange_time = '') AND (exchange_rules.exchange_time_1 = ? OR exchange_rules.exchange_time_2 = ?))
		)`, timeStr, timeStr, timeStr).
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
		if ok, reason := ShouldRunExchangeTaskAtWithCalendar(task, now, r.lookupCalendarHoliday); ok {
			if task.SkipReason != "" {
				_ = r.UpdateSkipReason(task.ID, "")
				task.SkipReason = ""
			}
			runnable = append(runnable, task)
		} else {
			_ = r.UpdateSkipReason(task.ID, reason)
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
