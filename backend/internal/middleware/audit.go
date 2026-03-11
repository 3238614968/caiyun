package middleware

import (
	"bytes"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"io"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditMiddleware 审计日志中间件
func AuditMiddleware(auditRepo *repository.AuditLogRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录开始时间
		startTime := time.Now()

		// 获取用户信息
		userID, _ := c.Get("user_id")
		username, _ := c.Get("username")

		// 读取请求体
		var requestBody []byte
		if c.Request.Body != nil {
			requestBody, _ = io.ReadAll(c.Request.Body)
			// 重新设置请求体，以便后续处理
			c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		}

		// 创建响应写入器来捕获响应
		blw := &bodyLogWriter{body: bytes.NewBufferString(""), ResponseWriter: c.Writer}
		c.Writer = blw

		// 继续处理请求
		c.Next()

		// 计算执行时间
		execTime := time.Since(startTime).Milliseconds()

		// 确定操作类型和资源
		action, resource := determineActionAndResource(c.Request.Method, c.Request.URL.Path)

		// 构建审计日志
		auditLog := &models.AuditLog{
			UserID:       getUintValue(userID),
			Username:     getStringValue(username),
			Action:       string(action),
			Resource:     string(resource),
			Method:       c.Request.Method,
			Path:         c.Request.URL.Path,
			IP:           c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
			RequestData:  truncateString(string(requestBody), 2000),
			ResponseData: truncateString(blw.body.String(), 2000),
			StatusCode:   c.Writer.Status(),
			ExecTimeMs:   int(execTime),
		}

		// 如果有错误，记录错误信息
		if len(c.Errors) > 0 {
			auditLog.ErrorMsg = c.Errors.String()
		}

		// 异步保存审计日志
		go func() {
			if err := auditRepo.Create(auditLog); err != nil {
				// 记录到系统日志，但不影响主流程
				gin.DefaultErrorWriter.Write([]byte("保存审计日志失败: " + err.Error() + "\n"))
			}
		}()
	}
}

// bodyLogWriter 用于捕获响应体的写入器
type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// determineActionAndResource 根据请求方法和路径确定操作类型和资源
func determineActionAndResource(method, path string) (models.AuditAction, models.AuditResource) {
	// 默认操作和资源
	action := models.AuditAction("UNKNOWN")
	resource := models.AuditResource("UNKNOWN")

	// 根据路径判断资源
	switch {
		case contains(path, "/accounts"):
			resource = models.AuditResourceAccount
		case contains(path, "/tasks"):
			resource = models.AuditResourceTask
		case contains(path, "/products"):
			resource = models.AuditResourceProduct
		case contains(path, "/exchange"):
			resource = models.AuditResourceExchange
		case contains(path, "/auth"):
			resource = models.AuditResourceUser
		case contains(path, "/config"):
			resource = models.AuditResourceConfig
	}

	// 根据方法和路径判断操作
	switch method {
		case "POST":
			if contains(path, "/login") {
				action = models.AuditActionLogin
			} else if contains(path, "/register") {
				action = models.AuditActionRegister
			} else if contains(path, "/accounts") {
				action = models.AuditActionCreateAccount
			} else if contains(path, "/tasks") {
				if contains(path, "/execute") {
					action = models.AuditActionExecuteTask
				} else {
					action = models.AuditActionCreateTask
				}
			} else if contains(path, "/exchange") {
				action = models.AuditActionExchange
			}
		case "PUT":
			if contains(path, "/accounts") {
				action = models.AuditActionUpdateAccount
			} else if contains(path, "/tasks") {
				action = models.AuditActionUpdateTask
			} else if contains(path, "/config") {
				action = models.AuditActionUpdateConfig
			} else if contains(path, "/profile") {
				action = models.AuditActionUpdateProfile
			}
		case "DELETE":
			if contains(path, "/accounts") {
				action = models.AuditActionDeleteAccount
			} else if contains(path, "/tasks") {
				action = models.AuditActionDeleteTask
			}
		case "GET":
			if contains(path, "/products") {
				action = models.AuditActionSearchProducts
			}
	}

	return action, resource
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// getUintValue 安全地获取uint值
func getUintValue(v interface{}) uint {
	if v == nil {
		return 0
	}
	if id, ok := v.(uint); ok {
		return id
	}
	return 0
}

// getStringValue 安全地获取string值
func getStringValue(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// AuditLogFilter 审计日志过滤器（用于排除某些路径）
type AuditLogFilter struct {
	ExcludedPaths []string
}

// NewAuditLogFilter 创建审计日志过滤器
func NewAuditLogFilter() *AuditLogFilter {
	return &AuditLogFilter{
		ExcludedPaths: []string{
			"/health",
			"/ws",
			"/api/auth/refresh",
		},
	}
}

// ShouldLog 检查是否应该记录审计日志
func (f *AuditLogFilter) ShouldLog(path string) bool {
	for _, excluded := range f.ExcludedPaths {
		if path == excluded || contains(path, excluded) {
			return false
		}
	}
	return true
}

// AuditMiddlewareWithFilter 带过滤器的审计日志中间件
func AuditMiddlewareWithFilter(auditRepo *repository.AuditLogRepository, filter *AuditLogFilter) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 检查是否应该记录
		if !filter.ShouldLog(c.Request.URL.Path) {
			c.Next()
			return
		}

		// 使用普通的审计中间件
		AuditMiddleware(auditRepo)(c)
	}
}
