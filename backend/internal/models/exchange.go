package models

import (
	"time"

	"gorm.io/gorm"
)

// Product 商品模型
type Product struct {
	ID                  uint           `gorm:"primarykey" json:"id"`
	PrizeID             string         `gorm:"uniqueIndex;size:100;not null" json:"prize_id"`         // 商品 ID(来自 API)
	PrizeName           string         `gorm:"column:prize_name;size:255;not null" json:"prize_name"` // 商品名称
	POrder              int            `gorm:"default:0" json:"p_order"`                              // 云朵价格
	Category            string         `gorm:"size:100;default:'未知分类'" json:"category"`               // 商品分类
	DailyRemainderCount int            `gorm:"default:0" json:"daily_remainder_count"`                // 每日剩余数量
	DailyLimitCount     int            `gorm:"default:0" json:"daily_limit_count"`                    // 每日限购数量
	DailyCount          int            `gorm:"default:0" json:"daily_count"`                          // 每日发放上限
	ImageURL            string         `gorm:"column:image_url;size:500;default:''" json:"image_url"` // 商品图片URL
	StockStatus         string         `gorm:"size:20;default:'unknown'" json:"stock_status"`         // 库存状态
	LastStockCheck      *time.Time     `json:"last_stock_check,omitempty"`                            // 最后库存检查时间
	Memo                string         `gorm:"type:text" json:"memo"`                                 // 商品备注信息
	IsActive            bool           `gorm:"default:true" json:"is_active"`                         // 是否启用
	IsDeleted           bool           `gorm:"default:false" json:"is_deleted"`                       // 是否已删除（软删除标记）
	UpdatedAt           time.Time      `json:"updated_at"`
	CreatedAt           time.Time      `json:"created_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

// ExchangeRule 抢兑规则模型。
//
// 历史 API/前端仍会看到 exchange_account_id / exchange_account 这些兼容字段，
// 但物理表已经统一为 exchange_rules，任务和记录通过 exchange_rule_id 关联。
type ExchangeRule struct {
	ID             uint           `gorm:"primarykey" json:"id"`
	UserID         uint           `gorm:"not null;index" json:"user_id"`                                              // 用户 ID
	AccountID      uint           `gorm:"not null;index" json:"account_id"`                                           // 云盘账号 ID
	Phone          string         `gorm:"size:20;not null" json:"phone"`                                              // 手机号
	Auth           string         `gorm:"type:text;not null" json:"-"`                                                // Basic Auth 字符串
	Token          string         `gorm:"type:text" json:"-"`                                                         // Token
	JWTToken       string         `gorm:"type:text" column:"jwt_token" json:"-"`                                      // JWT Token
	Remark         string         `gorm:"size:200" json:"remark"`                                                     // 备注
	ExchangeTime1  string         `gorm:"column:exchange_time_1;type:time;default:'10:00:00'" json:"exchange_time_1"` // 第一次抢兑时间
	ExchangeTime2  string         `gorm:"column:exchange_time_2;type:time;default:'16:00:00'" json:"exchange_time_2"` // 第二次抢兑时间
	IsActive       bool           `gorm:"default:true" json:"is_active"`                                              // 是否启用
	LastExchangeAt *time.Time     `json:"last_exchange_at,omitempty"`                                                 // 最后抢兑时间
	UpdatedAt      time.Time      `json:"updated_at"`
	CreatedAt      time.Time      `json:"created_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
	User           User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Account        Account        `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	Tasks          []ExchangeTask `gorm:"foreignKey:ExchangeAccountID" json:"tasks,omitempty"`
}

func (ExchangeRule) TableName() string {
	return "exchange_rules"
}

// ExchangeAccount 是历史兼容别名；新代码优先使用 ExchangeRule。
type ExchangeAccount = ExchangeRule

// ExchangeTask 抢兑任务模型
type ExchangeTask struct {
	ID                    uint       `gorm:"primarykey" json:"id"`
	SourceOperationID     *string    `gorm:"column:source_operation_id;size:36;uniqueIndex" json:"-"`
	UserID                uint       `gorm:"not null;index" json:"user_id"`                                                     // 用户 ID
	ExchangeAccountID     uint       `gorm:"column:exchange_rule_id;not null;index" json:"exchange_account_id"`                 // 抢兑规则 ID（历史 JSON 字段名保留）
	ProductID             uint       `gorm:"not null;index" json:"product_id"`                                                  // 商品 ID
	PrizeID               string     `gorm:"size:100;not null" json:"prize_id"`                                                 // 商品 ID(来自 API)
	PrizeName             string     `gorm:"size:255;not null" json:"prize_name"`                                               // 商品名称
	TaskType              string     `gorm:"size:20;default:'fixed'" json:"task_type"`                                          // 任务类型：fixed=固定次数，long_term=长期抢兑
	MaxAttempts           int        `gorm:"default:1" json:"max_attempts"`                                                     // 最大抢兑次数 (固定次数模式)
	ScheduledExchangeTime string     `gorm:"column:scheduled_exchange_time;type:time" json:"scheduled_exchange_time,omitempty"` // 任务级指定抢兑时间；为空时使用账号规则时间
	RestockCycle          string     `gorm:"size:20;default:'daily'" json:"restock_cycle"`                                      // 补货周期：daily/weekly/monthly/once
	RestockWeekday        *int       `json:"restock_weekday,omitempty"`                                                         // 周期为 weekly 时的星期，0=周日
	RestockDayOfMonth     *int       `json:"restock_day_of_month,omitempty"`                                                    // 周期为 monthly 时的日期
	RestockTimes          string     `gorm:"type:text" json:"restock_times,omitempty"`                                          // 补货时间点列表，逗号分隔 HH:MM[:SS]
	CustomCron            string     `gorm:"size:120;default:''" json:"custom_cron,omitempty"`                                  // 自定义 cron，优先于固定时间/补货时间
	CalendarPolicy        string     `gorm:"size:20;default:'all'" json:"calendar_policy"`                                      // 日历策略：all/workday/holiday
	HolidayDates          string     `gorm:"type:text" json:"holiday_dates,omitempty"`                                          // 额外节假日 YYYY-MM-DD，逗号分隔
	WorkdayDates          string     `gorm:"type:text" json:"workday_dates,omitempty"`                                          // 调休工作日 YYYY-MM-DD，逗号分隔
	SkipReason            string     `gorm:"size:255;default:''" json:"skip_reason,omitempty"`                                  // 最近一次调度跳过原因
	AttemptedCount        int        `gorm:"default:0" json:"attempted_count"`                                                  // 已抢兑次数
	Status                string     `gorm:"size:20;default:'pending'" json:"status"`                                           // 任务状态
	LastAttemptAt         *time.Time `json:"last_attempt_at,omitempty"`                                                         // 最后抢兑时间
	LastResult            string     `gorm:"type:text" json:"last_result"`                                                      // 最后抢兑结果
	NextRunAt             *time.Time `gorm:"-" json:"next_run_at,omitempty"`                                                    // 下一次预计触发时间（只读预览）
	// 新增字段：优先级、分组、超时控制
	Priority        int              `gorm:"default:0" json:"priority"`            // 优先级：0-普通，1-重要，2-紧急
	TaskGroup       string           `gorm:"size:50;default:''" json:"task_group"` // 任务分组标识
	TimeoutSeconds  int              `gorm:"default:30" json:"timeout_seconds"`    // 超时时间（秒）
	MaxRetries      int              `gorm:"default:3" json:"max_retries"`         // 最大重试次数
	RetryCount      int              `gorm:"default:0" json:"retry_count"`         // 已重试次数
	LastRetryAt     *time.Time       `json:"last_retry_at,omitempty"`              // 最后重试时间
	SuccessCount    int              `gorm:"default:0" json:"success_count"`       // 成功次数
	FailCount       int              `gorm:"default:0" json:"fail_count"`          // 失败次数
	UpdatedAt       time.Time        `json:"updated_at"`
	CreatedAt       time.Time        `json:"created_at"`
	DeletedAt       gorm.DeletedAt   `gorm:"index" json:"-"`
	User            User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	ExchangeAccount ExchangeAccount  `gorm:"foreignKey:ExchangeAccountID" json:"exchange_account,omitempty"`
	Product         Product          `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Records         []ExchangeRecord `gorm:"foreignKey:ExchangeTaskID" json:"records,omitempty"`
}

// ExchangeRecord 抢兑记录模型
type ExchangeRecord struct {
	ID                uint      `gorm:"primarykey" json:"id"`
	UserID            uint      `gorm:"not null;index" json:"user_id"`                                     // 用户 ID
	ExchangeAccountID uint      `gorm:"column:exchange_rule_id;not null;index" json:"exchange_account_id"` // 抢兑规则 ID（历史 JSON 字段名保留）
	ExchangeTaskID    *uint     `json:"exchange_task_id,omitempty"`                                        // 抢兑任务 ID
	ProductID         uint      `gorm:"not null;index" json:"product_id"`                                  // 商品 ID
	PrizeID           string    `gorm:"size:100;not null" json:"prize_id"`                                 // 商品 ID(来自 API)
	PrizeName         string    `gorm:"size:255;not null" json:"prize_name"`                               // 商品名称
	Status            string    `gorm:"size:20;not null" json:"status"`                                    // 抢兑结果
	Message           string    `gorm:"type:text" json:"message"`                                          // 抢兑结果消息
	ExecutionTimeMs   int       `gorm:"default:0" json:"execution_time_ms"`                                // 执行时长 (毫秒)
	CreatedAt         time.Time `gorm:"index" json:"created_at"`

	ExchangeAccount *ExchangeAccount `gorm:"foreignKey:ExchangeAccountID" json:"exchange_account,omitempty"`
	Product         *Product         `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

// CalendarDate 节假日/调休工作日模型。
// date 使用 DATE 主键，day_type 取值 holiday/workday；调度策略优先读取该表，
// 表中未覆盖的日期再回退到周末判断，便于逐年导入真实国务院节假日安排。
type CalendarDate struct {
	Date      string    `gorm:"primaryKey;type:date" json:"date"`
	DayType   string    `gorm:"column:day_type;size:20;not null;index" json:"day_type"`
	Name      string    `gorm:"size:100;default:''" json:"name"`
	Source    string    `gorm:"size:120;default:''" json:"source"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (CalendarDate) TableName() string {
	return "calendar_dates"
}

// ExchangeTaskStatus 抢兑任务状态枚举
type ExchangeTaskStatus string

const (
	ExchangeTaskPending   ExchangeTaskStatus = "pending"
	ExchangeTaskRunning   ExchangeTaskStatus = "running"
	ExchangeTaskCompleted ExchangeTaskStatus = "completed"
	ExchangeTaskFailed    ExchangeTaskStatus = "failed"
)

// ExchangeTaskType 抢兑任务类型枚举
type ExchangeTaskType string

const (
	ExchangeTaskFixed    ExchangeTaskType = "fixed"
	ExchangeTaskLongTerm ExchangeTaskType = "long_term"
)

// ExchangeRecordStatus 抢兑记录状态枚举
type ExchangeRecordStatus string

const (
	ExchangeRecordSuccess ExchangeRecordStatus = "success"
	ExchangeRecordFailed  ExchangeRecordStatus = "failed"
)

// WebSocketMessage WebSocket消息持久化模型
type WebSocketMessage struct {
	ID          uint       `gorm:"primarykey" json:"id"`
	UserID      uint       `gorm:"not null;index;index:idx_ws_user_sequence,priority:1" json:"user_id"`
	MessageID   string     `gorm:"column:message_id;size:64;uniqueIndex" json:"message_id"`
	Sequence    uint64     `gorm:"index:idx_ws_user_sequence,priority:2" json:"sequence"`
	Type        string     `gorm:"size:50;not null" json:"type"`
	Data        string     `gorm:"type:text" json:"data"`
	IsRead      bool       `gorm:"default:false" json:"is_read"`
	IsDelivered bool       `gorm:"default:false" json:"is_delivered"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
	AckedAt     *time.Time `json:"acked_at,omitempty"`
}
