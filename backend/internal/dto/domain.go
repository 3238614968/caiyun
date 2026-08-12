package dto

import (
	"caiyun/internal/models"
	"time"
)

// ProductResponse is the public product catalog view.
type ProductResponse struct {
	ID                  uint       `json:"id"`
	PrizeID             string     `json:"prize_id"`
	PrizeName           string     `json:"prize_name"`
	POrder              int        `json:"p_order"`
	Category            string     `json:"category"`
	DailyRemainderCount int        `json:"daily_remainder_count"`
	DailyLimitCount     int        `json:"daily_limit_count"`
	DailyCount          int        `json:"daily_count"`
	ImageURL            string     `json:"image_url"`
	StockStatus         string     `json:"stock_status"`
	LastStockCheck      *time.Time `json:"last_stock_check,omitempty"`
	Memo                string     `json:"memo"`
	IsActive            bool       `json:"is_active"`
	IsDeleted           bool       `json:"is_deleted"`
	UpdatedAt           time.Time  `json:"updated_at"`
	CreatedAt           time.Time  `json:"created_at"`
}

func ToProductResponse(product *models.Product) *ProductResponse {
	if product == nil {
		return nil
	}
	return &ProductResponse{ID: product.ID, PrizeID: product.PrizeID, PrizeName: product.PrizeName, POrder: product.POrder, Category: product.Category, DailyRemainderCount: product.DailyRemainderCount, DailyLimitCount: product.DailyLimitCount, DailyCount: product.DailyCount, ImageURL: product.ImageURL, StockStatus: product.StockStatus, LastStockCheck: product.LastStockCheck, Memo: product.Memo, IsActive: product.IsActive, IsDeleted: product.IsDeleted, UpdatedAt: product.UpdatedAt, CreatedAt: product.CreatedAt}
}

func ToProductResponses(products []*models.Product) []*ProductResponse {
	result := make([]*ProductResponse, len(products))
	for i, product := range products {
		result[i] = ToProductResponse(product)
	}
	return result
}

// ExchangeRuleResponse keeps the legacy exchange_account field semantics while
// excluding credential fields and nested task collections.
type ExchangeRuleResponse struct {
	ID             uint             `json:"id"`
	UserID         uint             `json:"user_id"`
	AccountID      uint             `json:"account_id"`
	Phone          string           `json:"phone"`
	Remark         string           `json:"remark"`
	ExchangeTime1  string           `json:"exchange_time_1"`
	ExchangeTime2  string           `json:"exchange_time_2"`
	IsActive       bool             `json:"is_active"`
	LastExchangeAt *time.Time       `json:"last_exchange_at,omitempty"`
	UpdatedAt      time.Time        `json:"updated_at"`
	CreatedAt      time.Time        `json:"created_at"`
	User           *UserSummary     `json:"user,omitempty"`
	Account        *AccountResponse `json:"account,omitempty"`
}

func ToExchangeRuleResponse(rule *models.ExchangeRule) *ExchangeRuleResponse {
	if rule == nil {
		return nil
	}
	response := &ExchangeRuleResponse{ID: rule.ID, UserID: rule.UserID, AccountID: rule.AccountID, Phone: rule.Phone, Remark: rule.Remark, ExchangeTime1: rule.ExchangeTime1, ExchangeTime2: rule.ExchangeTime2, IsActive: rule.IsActive, LastExchangeAt: rule.LastExchangeAt, UpdatedAt: rule.UpdatedAt, CreatedAt: rule.CreatedAt, Account: ToAccountResponse(&rule.Account)}
	if rule.User.ID != 0 {
		response.User = &UserSummary{ID: rule.User.ID, Username: rule.User.Username, Email: rule.User.Email, Role: rule.User.Role}
	}
	return response
}

func ToExchangeRuleResponses(rules []*models.ExchangeRule) []*ExchangeRuleResponse {
	result := make([]*ExchangeRuleResponse, len(rules))
	for i, rule := range rules {
		result[i] = ToExchangeRuleResponse(rule)
	}
	return result
}

// TaskLogResponse contains task history without serializing an entire GORM
// association graph.
type TaskLogResponse struct {
	ID            uint             `json:"id"`
	UserID        uint             `json:"user_id"`
	AccountID     uint             `json:"account_id"`
	TaskType      string           `json:"task_type"`
	Status        string           `json:"status"`
	Message       string           `json:"message"`
	CloudGained   int              `json:"cloud_gained"`
	ExecutionTime int              `json:"execution_time"`
	CreatedAt     time.Time        `json:"created_at"`
	Account       *AccountResponse `json:"account,omitempty"`
}

func ToTaskLogResponse(log *models.TaskLog) *TaskLogResponse {
	if log == nil {
		return nil
	}
	return &TaskLogResponse{ID: log.ID, UserID: log.UserID, AccountID: log.AccountID, TaskType: log.TaskType, Status: log.Status, Message: log.Message, CloudGained: log.CloudGained, ExecutionTime: log.ExecutionTime, CreatedAt: log.CreatedAt, Account: ToAccountResponse(&log.Account)}
}
func ToTaskLogResponses(logs []*models.TaskLog) []*TaskLogResponse {
	result := make([]*TaskLogResponse, len(logs))
	for i, item := range logs {
		result[i] = ToTaskLogResponse(item)
	}
	return result
}

// CloudStatsResponse is the stable historical cloud-count view.
type CloudStatsResponse struct {
	ID            uint             `json:"id"`
	UserID        uint             `json:"user_id"`
	AccountID     uint             `json:"account_id"`
	Date          string           `json:"date"`
	CloudCount    int              `json:"cloud_count"`
	CloudDiff     int              `json:"cloud_diff"`
	CloudDiffWeek int              `json:"cloud_diff_week"`
	CreatedAt     time.Time        `json:"created_at"`
	UpdatedAt     time.Time        `json:"updated_at"`
	Account       *AccountResponse `json:"account,omitempty"`
}

func ToCloudStatsResponse(stats *models.CloudStats) *CloudStatsResponse {
	if stats == nil {
		return nil
	}
	return &CloudStatsResponse{ID: stats.ID, UserID: stats.UserID, AccountID: stats.AccountID, Date: stats.Date, CloudCount: stats.CloudCount, CloudDiff: stats.CloudDiff, CloudDiffWeek: stats.CloudDiffWeek, CreatedAt: stats.CreatedAt, UpdatedAt: stats.UpdatedAt, Account: ToAccountResponse(&stats.Account)}
}
func ToCloudStatsResponses(stats []*models.CloudStats) []*CloudStatsResponse {
	result := make([]*CloudStatsResponse, len(stats))
	for i, item := range stats {
		result[i] = ToCloudStatsResponse(item)
	}
	return result
}

// ExchangeTaskResponse excludes execution ownership and source-operation
// internals while preserving the established public scheduling shape.
type ExchangeTaskResponse struct {
	ID                    uint                  `json:"id"`
	UserID                uint                  `json:"user_id"`
	ExchangeAccountID     uint                  `json:"exchange_account_id"`
	ProductID             uint                  `json:"product_id"`
	PrizeID               string                `json:"prize_id"`
	PrizeName             string                `json:"prize_name"`
	TaskType              string                `json:"task_type"`
	MaxAttempts           int                   `json:"max_attempts"`
	ScheduledExchangeTime string                `json:"scheduled_exchange_time,omitempty"`
	RestockCycle          string                `json:"restock_cycle"`
	RestockWeekday        *int                  `json:"restock_weekday,omitempty"`
	RestockDayOfMonth     *int                  `json:"restock_day_of_month,omitempty"`
	RestockTimes          string                `json:"restock_times,omitempty"`
	CustomCron            string                `json:"custom_cron,omitempty"`
	CalendarPolicy        string                `json:"calendar_policy"`
	HolidayDates          string                `json:"holiday_dates,omitempty"`
	WorkdayDates          string                `json:"workday_dates,omitempty"`
	SkipReason            string                `json:"skip_reason,omitempty"`
	AttemptedCount        int                   `json:"attempted_count"`
	Status                string                `json:"status"`
	LastAttemptAt         *time.Time            `json:"last_attempt_at,omitempty"`
	LastResult            string                `json:"last_result"`
	NextRunAt             *time.Time            `json:"next_run_at,omitempty"`
	Priority              int                   `json:"priority"`
	TaskGroup             string                `json:"task_group"`
	TimeoutSeconds        int                   `json:"timeout_seconds"`
	MaxRetries            int                   `json:"max_retries"`
	RetryCount            int                   `json:"retry_count"`
	LastRetryAt           *time.Time            `json:"last_retry_at,omitempty"`
	SuccessCount          int                   `json:"success_count"`
	FailCount             int                   `json:"fail_count"`
	UpdatedAt             time.Time             `json:"updated_at"`
	CreatedAt             time.Time             `json:"created_at"`
	User                  *UserSummary          `json:"user,omitempty"`
	ExchangeAccount       *ExchangeRuleResponse `json:"exchange_account,omitempty"`
	Product               *ProductResponse      `json:"product,omitempty"`
}

func ToExchangeTaskResponse(task *models.ExchangeTask) *ExchangeTaskResponse {
	if task == nil {
		return nil
	}
	result := &ExchangeTaskResponse{ID: task.ID, UserID: task.UserID, ExchangeAccountID: task.ExchangeAccountID, ProductID: task.ProductID, PrizeID: task.PrizeID, PrizeName: task.PrizeName, TaskType: task.TaskType, MaxAttempts: task.MaxAttempts, ScheduledExchangeTime: task.ScheduledExchangeTime, RestockCycle: task.RestockCycle, RestockWeekday: task.RestockWeekday, RestockDayOfMonth: task.RestockDayOfMonth, RestockTimes: task.RestockTimes, CustomCron: task.CustomCron, CalendarPolicy: task.CalendarPolicy, HolidayDates: task.HolidayDates, WorkdayDates: task.WorkdayDates, SkipReason: task.SkipReason, AttemptedCount: task.AttemptedCount, Status: task.Status, LastAttemptAt: task.LastAttemptAt, LastResult: task.LastResult, NextRunAt: task.NextRunAt, Priority: task.Priority, TaskGroup: task.TaskGroup, TimeoutSeconds: task.TimeoutSeconds, MaxRetries: task.MaxRetries, RetryCount: task.RetryCount, LastRetryAt: task.LastRetryAt, SuccessCount: task.SuccessCount, FailCount: task.FailCount, UpdatedAt: task.UpdatedAt, CreatedAt: task.CreatedAt, ExchangeAccount: ToExchangeRuleResponse(&task.ExchangeAccount), Product: ToProductResponse(&task.Product)}
	if task.User.ID != 0 {
		result.User = &UserSummary{ID: task.User.ID, Username: task.User.Username, Email: task.User.Email, Role: task.User.Role}
	}
	return result
}
func ToExchangeTaskResponses(tasks []*models.ExchangeTask) []*ExchangeTaskResponse {
	result := make([]*ExchangeTaskResponse, len(tasks))
	for i, task := range tasks {
		result[i] = ToExchangeTaskResponse(task)
	}
	return result
}

// ExchangeRecordResponse is used for both list and JSON export output.
type ExchangeRecordResponse struct {
	ID                uint                  `json:"id"`
	UserID            uint                  `json:"user_id"`
	ExchangeAccountID uint                  `json:"exchange_account_id"`
	ExchangeTaskID    *uint                 `json:"exchange_task_id,omitempty"`
	ProductID         uint                  `json:"product_id"`
	PrizeID           string                `json:"prize_id"`
	PrizeName         string                `json:"prize_name"`
	Status            string                `json:"status"`
	Message           string                `json:"message"`
	ExecutionTimeMs   int                   `json:"execution_time_ms"`
	CreatedAt         time.Time             `json:"created_at"`
	ExchangeAccount   *ExchangeRuleResponse `json:"exchange_account,omitempty"`
	Product           *ProductResponse      `json:"product,omitempty"`
}

func ToExchangeRecordResponse(record *models.ExchangeRecord) *ExchangeRecordResponse {
	if record == nil {
		return nil
	}
	return &ExchangeRecordResponse{ID: record.ID, UserID: record.UserID, ExchangeAccountID: record.ExchangeAccountID, ExchangeTaskID: record.ExchangeTaskID, ProductID: record.ProductID, PrizeID: record.PrizeID, PrizeName: record.PrizeName, Status: record.Status, Message: record.Message, ExecutionTimeMs: record.ExecutionTimeMs, CreatedAt: record.CreatedAt, ExchangeAccount: ToExchangeRuleResponse(record.ExchangeAccount), Product: ToProductResponse(record.Product)}
}
func ToExchangeRecordResponses(records []*models.ExchangeRecord) []*ExchangeRecordResponse {
	result := make([]*ExchangeRecordResponse, len(records))
	for i, record := range records {
		result[i] = ToExchangeRecordResponse(record)
	}
	return result
}
