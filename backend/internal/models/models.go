package models

import (
	"time"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	Username     string         `gorm:"uniqueIndex;size:50;not null" json:"username"`
	Password     string         `gorm:"size:255;not null" json:"-"`
	Email        string         `gorm:"size:100" json:"email"`
	Role         string         `gorm:"default:'user';size:10" json:"role"` // user, admin
	TokenVersion int            `gorm:"not null;default:0" json:"-"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	Accounts     []Account      `gorm:"foreignKey:UserID" json:"accounts,omitempty"`
}

// Account 云盘账号模型
type Account struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	Phone         string         `gorm:"size:20;not null" json:"phone"`
	Auth          string         `gorm:"type:text;not null" json:"-"`
	Token         string         `gorm:"type:text" json:"-"`
	JWTToken      string         `gorm:"type:text" column:"jwt_token" json:"-"`
	Platform      string         `gorm:"default:'pc';size:20" json:"platform"`
	ExpireAt      int64          `gorm:"index" json:"expire_at"`
	CloudCount    int            `gorm:"default:0" json:"cloud_count"`
	Remark        string         `gorm:"size:200" json:"remark"`
	IsActive      bool           `gorm:"default:true" json:"is_active"`
	JWTErrorCount int            `gorm:"default:0" column:"jwt_error_count" json:"jwt_error_count"` // JWT获取失败次数
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	User          User           `gorm:"foreignKey:UserID" json:"user,omitempty"`
	TaskLogs      []TaskLog      `gorm:"foreignKey:AccountID" json:"task_logs,omitempty"`
}

// TaskLog 任务日志模型
type TaskLog struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	AccountID     uint           `gorm:"not null;index" json:"account_id"`
	TaskType      string         `gorm:"size:50;not null" json:"task_type"`       // signin, wechat, shake, etc.
	Status        string         `gorm:"default:'pending';size:20" json:"status"` // success, failed, pending
	Message       string         `gorm:"type:text" json:"message"`
	CloudGained   int            `gorm:"default:0" json:"cloud_gained"`
	ExecutionTime int            `gorm:"default:0" json:"execution_time"` // 毫秒
	CreatedAt     time.Time      `gorm:"index" json:"created_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	Account       Account        `gorm:"foreignKey:AccountID" json:"account,omitempty"`
}

// CloudStats 云朵统计模型
type CloudStats struct {
	ID            uint           `gorm:"primarykey" json:"id"`
	UserID        uint           `gorm:"not null;index" json:"user_id"`
	AccountID     uint           `gorm:"not null;index" json:"account_id"`
	Date          string         `gorm:"type:date;not null" json:"date"`
	CloudCount    int            `gorm:"not null" json:"cloud_count"`
	CloudDiff     int            `gorm:"default:0" json:"cloud_diff"`      // 对比昨日
	CloudDiffWeek int            `gorm:"default:0" json:"cloud_diff_week"` // 对比上周
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	Account       Account        `gorm:"foreignKey:AccountID" json:"account,omitempty"`
}

// SystemConfig 系统配置模型
type SystemConfig struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	KeyName     string    `gorm:"uniqueIndex;size:100;not null" json:"key_name"`
	KeyValue    string    `gorm:"type:text;not null" json:"key_value"`
	Description string    `gorm:"size:200" json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TaskConfig 任务配置模型（注册表同步后的可维护任务定义）
type TaskConfig struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	TaskType    string         `gorm:"uniqueIndex;size:50;not null" json:"task_type"`
	TaskName    string         `gorm:"size:50;not null" json:"task_name"`
	Description string         `gorm:"size:255" json:"description"`
	IsEnabled   bool           `gorm:"default:true" json:"is_enabled"`
	SortOrder   int            `gorm:"default:0" json:"sort_order"`
	RunInBatch  bool           `gorm:"default:true" json:"run_in_batch"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
