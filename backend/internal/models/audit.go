package models

import (
	"time"
)

// AuditLog 审计日志模型
type AuditLog struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	UserID       uint      `gorm:"not null;index" json:"user_id"`            // 用户ID
	Username     string    `gorm:"size:100" json:"username"`                 // 用户名
	Action       string    `gorm:"size:50;not null;index" json:"action"`     // 操作类型
	Resource     string    `gorm:"size:100;not null" json:"resource"`        // 资源类型
	ResourceID   string    `gorm:"size:50" json:"resource_id"`               // 资源ID
	Method       string    `gorm:"size:10" json:"method"`                    // HTTP方法
	Path         string    `gorm:"size:500" json:"path"`                     // 请求路径
	IP           string    `gorm:"size:50" json:"ip"`                        // 客户端IP
	UserAgent    string    `gorm:"size:500" json:"user_agent"`               // 用户代理
	RequestData  string    `gorm:"type:text" json:"request_data,omitempty"`  // 请求数据(JSON)
	ResponseData string    `gorm:"type:text" json:"response_data,omitempty"` // 响应数据(JSON)
	StatusCode   int       `gorm:"default:0" json:"status_code"`             // HTTP状态码
	ErrorMsg     string    `gorm:"type:text" json:"error_msg,omitempty"`     // 错误信息
	ExecTimeMs   int       `gorm:"default:0" json:"exec_time_ms"`            // 执行时长(毫秒)
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// AuditAction 审计操作类型
type AuditAction string

const (
	// 账号相关
	AuditActionCreateAccount AuditAction = "CREATE_ACCOUNT"
	AuditActionUpdateAccount AuditAction = "UPDATE_ACCOUNT"
	AuditActionDeleteAccount AuditAction = "DELETE_ACCOUNT"

	// 任务相关
	AuditActionCreateTask  AuditAction = "CREATE_TASK"
	AuditActionUpdateTask  AuditAction = "UPDATE_TASK"
	AuditActionDeleteTask  AuditAction = "DELETE_TASK"
	AuditActionExecuteTask AuditAction = "EXECUTE_TASK"

	// 兑换相关
	AuditActionExchange AuditAction = "EXCHANGE"

	// 商品相关
	AuditActionSearchProducts AuditAction = "SEARCH_PRODUCTS"

	// 配置相关
	AuditActionUpdateConfig AuditAction = "UPDATE_CONFIG"

	// 用户相关
	AuditActionLogin         AuditAction = "LOGIN"
	AuditActionRegister      AuditAction = "REGISTER"
	AuditActionUpdateProfile AuditAction = "UPDATE_PROFILE"
)

// AuditResource 审计资源类型
type AuditResource string

const (
	AuditResourceAccount  AuditResource = "ACCOUNT"
	AuditResourceTask     AuditResource = "TASK"
	AuditResourceProduct  AuditResource = "PRODUCT"
	AuditResourceConfig   AuditResource = "CONFIG"
	AuditResourceUser     AuditResource = "USER"
	AuditResourceExchange AuditResource = "EXCHANGE"
)

// TableName 指定表名
func (AuditLog) TableName() string {
	return "audit_logs"
}
