package repository

import (
	"caiyun/internal/models"
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrExchangeTaskExecutionLost = errors.New("exchange task execution ownership lost")

// ExchangeTaskRepository 抢兑任务数据访问层
type ExchangeTaskRepository struct {
	db *gorm.DB
}

// ExchangeTaskFilter 描述抢兑任务列表筛选条件。
type ExchangeTaskFilter struct {
	AccountKeyword string
	Remark         string
	Status         string
	RestockCycle   string
	MinCloud       *int
	MaxCloud       *int
	OnlyActive     *bool
}

func NewExchangeTaskRepository(db *gorm.DB) *ExchangeTaskRepository {
	return &ExchangeTaskRepository{db: db}
}

// CalculateNextRun 使用真实节假日表计算任务下一次预计触发时间。
func (r *ExchangeTaskRepository) CalculateNextRun(task *models.ExchangeTask, from time.Time) *time.Time {
	return CalculateExchangeTaskNextRunWithCalendar(task, from, r.lookupCalendarHoliday)
}

func (r *ExchangeTaskRepository) lookupCalendarHoliday(now time.Time) (bool, bool) {
	if r == nil || r.db == nil {
		return false, false
	}
	var row struct {
		DayType string `gorm:"column:day_type"`
	}
	result := r.db.Table("calendar_dates").
		Select("day_type").
		Where("date = ?", now.Format("2006-01-02")).
		Limit(1).
		Scan(&row)
	if result.Error != nil || result.RowsAffected == 0 {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(row.DayType)) {
	case "holiday", "off", "rest", "节假日", "休息日":
		return true, true
	case "workday", "working_day", "work", "调休工作日", "工作日":
		return false, true
	default:
		return false, false
	}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *ExchangeTaskRepository) WithContext(ctx context.Context) *ExchangeTaskRepository {
	if ctx == nil {
		return r
	}
	return &ExchangeTaskRepository{db: r.db.WithContext(ctx)}
}
