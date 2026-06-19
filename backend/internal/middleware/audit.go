package middleware

import (
	"bytes"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	auditWorkerCount = 4
	auditBufferSize  = 4096
)

// asyncAuditWriter 是应用级单例，所有审计中间件共享同一组 worker，
// 避免每个请求都新建 worker goroutine 造成泄漏。
type asyncAuditWriter struct {
	repo     *repository.AuditLogRepository
	ch       chan *models.AuditLog
	stopOnce sync.Once
	stopCh   chan struct{}
}

var (
	globalAuditWriter   *asyncAuditWriter
	globalAuditWriterMu sync.Mutex
	auditDroppedTotal   atomic.Int64
)

// InitGlobalAuditWriter 在应用启动时调用一次，创建共享的审计 writer 并启动 worker。
// 可重复调用：若 repo 变化会重新创建（主要用于测试与未来热替换场景）。
func InitGlobalAuditWriter(repo *repository.AuditLogRepository) {
	globalAuditWriterMu.Lock()
	defer globalAuditWriterMu.Unlock()

	// 已经创建过且 repo 未变化则跳过。
	if globalAuditWriter != nil && globalAuditWriter.repo == repo {
		return
	}
	// 关闭旧实例（如有）。
	if globalAuditWriter != nil {
		globalAuditWriter.stop()
	}
	globalAuditWriter = newAsyncAuditWriter(repo)
}

// StopGlobalAuditWriter 在应用优雅退出时调用，关闭 worker。
func StopGlobalAuditWriter() {
	globalAuditWriterMu.Lock()
	defer globalAuditWriterMu.Unlock()
	if globalAuditWriter != nil {
		globalAuditWriter.stop()
		globalAuditWriter = nil
	}
}

func newAsyncAuditWriter(repo *repository.AuditLogRepository) *asyncAuditWriter {
	w := &asyncAuditWriter{
		repo:   repo,
		ch:     make(chan *models.AuditLog, auditBufferSize),
		stopCh: make(chan struct{}),
	}
	for i := 0; i < auditWorkerCount; i++ {
		go w.worker()
	}
	return w
}

func (w *asyncAuditWriter) stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stopCh)
		close(w.ch)
	})
}

func (w *asyncAuditWriter) worker() {
	for {
		select {
		case auditLog, ok := <-w.ch:
			if !ok {
				return
			}
			if auditLog == nil || w.repo == nil {
				continue
			}
			if err := w.repo.Create(auditLog); err != nil {
				gin.DefaultErrorWriter.Write([]byte("保存审计日志失败: " + err.Error() + "\n"))
			}
		case <-w.stopCh:
			return
		}
	}
}

func (w *asyncAuditWriter) enqueue(auditLog *models.AuditLog) {
	if w == nil || auditLog == nil {
		return
	}
	select {
	case w.ch <- auditLog:
	default:
		auditDroppedTotal.Add(1)
		gin.DefaultErrorWriter.Write([]byte("审计日志队列已满，丢弃当前审计日志\n"))
	}
}

// AuditDroppedCount 返回因异步队列满而丢弃的审计日志累计数量。
func AuditDroppedCount() int64 {
	return auditDroppedTotal.Load()
}

// getGlobalAuditWriter 返回已初始化的全局审计 writer；
// 若调用方未显式初始化（例如测试），则惰性返回 nil 安全处理。
func getGlobalAuditWriter(repo *repository.AuditLogRepository) *asyncAuditWriter {
	globalAuditWriterMu.Lock()
	defer globalAuditWriterMu.Unlock()
	if globalAuditWriter != nil {
		return globalAuditWriter
	}
	// 兜底：若未显式初始化，则惰性创建一次（保持向后兼容）。
	if repo != nil {
		globalAuditWriter = newAsyncAuditWriter(repo)
		return globalAuditWriter
	}
	return nil
}

// AuditMiddleware 审计日志中间件（使用全局共享 writer）。
func AuditMiddleware(auditRepo *repository.AuditLogRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		writer := getGlobalAuditWriter(auditRepo)
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
			RequestData:  truncateString(redactAuditPayload(requestBody), 2000),
			ResponseData: truncateString(redactAuditPayload(blw.body.Bytes()), 2000),
			StatusCode:   c.Writer.Status(),
			ExecTimeMs:   int(execTime),
		}

		// 如果有错误，记录错误信息
		if len(c.Errors) > 0 {
			auditLog.ErrorMsg = redactPlainAuditPayload(c.Errors.String())
		}

		if writer != nil {
			writer.enqueue(auditLog)
		}
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
	case strings.Contains(path, "/accounts"):
		resource = models.AuditResourceAccount
	case strings.Contains(path, "/tasks"):
		resource = models.AuditResourceTask
	case strings.Contains(path, "/products"):
		resource = models.AuditResourceProduct
	case strings.Contains(path, "/exchange"):
		resource = models.AuditResourceExchange
	case strings.Contains(path, "/auth"):
		resource = models.AuditResourceUser
	case strings.Contains(path, "/config"):
		resource = models.AuditResourceConfig
	}

	// 根据方法和路径判断操作
	switch method {
	case "POST":
		if strings.Contains(path, "/login") {
			action = models.AuditActionLogin
		} else if strings.Contains(path, "/register") {
			action = models.AuditActionRegister
		} else if strings.Contains(path, "/accounts") {
			action = models.AuditActionCreateAccount
		} else if strings.Contains(path, "/tasks") {
			if strings.Contains(path, "/execute") {
				action = models.AuditActionExecuteTask
			} else {
				action = models.AuditActionCreateTask
			}
		} else if strings.Contains(path, "/exchange") {
			action = models.AuditActionExchange
		}
	case "PUT":
		if strings.Contains(path, "/accounts") {
			action = models.AuditActionUpdateAccount
		} else if strings.Contains(path, "/tasks") {
			action = models.AuditActionUpdateTask
		} else if strings.Contains(path, "/config") {
			action = models.AuditActionUpdateConfig
		} else if strings.Contains(path, "/profile") {
			action = models.AuditActionUpdateProfile
		}
	case "DELETE":
		if strings.Contains(path, "/accounts") {
			action = models.AuditActionDeleteAccount
		} else if strings.Contains(path, "/tasks") {
			action = models.AuditActionDeleteTask
		}
	case "GET":
		if strings.Contains(path, "/products") {
			action = models.AuditActionSearchProducts
		}
	}

	return action, resource
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

// redactAuditPayload 脱敏审计日志中的请求/响应载荷，避免凭据二次落库。
func redactAuditPayload(payload []byte) string {
	payload = bytes.TrimSpace(payload)
	if len(payload) == 0 {
		return ""
	}

	var data interface{}
	if err := json.Unmarshal(payload, &data); err == nil {
		redacted := redactAuditValue(data)
		if encoded, err := json.Marshal(redacted); err == nil {
			return string(encoded)
		}
	}

	return redactPlainAuditPayload(string(payload))
}

func redactAuditValue(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, item := range v {
			if isSensitiveAuditKey(key) {
				v[key] = "[REDACTED]"
				continue
			}
			v[key] = redactAuditValue(item)
		}
		return v
	case []interface{}:
		for i, item := range v {
			v[i] = redactAuditValue(item)
		}
		return v
	default:
		return value
	}
}

func redactPlainAuditPayload(payload string) string {
	if payload == "" {
		return ""
	}
	lower := strings.ToLower(payload)
	for _, key := range sensitiveAuditKeys {
		if strings.Contains(lower, key) {
			return "[REDACTED_SENSITIVE_PAYLOAD]"
		}
	}
	return payload
}

func isSensitiveAuditKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	for _, sensitiveKey := range sensitiveAuditKeys {
		if strings.Contains(normalized, strings.ReplaceAll(sensitiveKey, "_", "")) {
			return true
		}
	}
	return false
}

var sensitiveAuditKeys = []string{
	"password",
	"auth",
	"authorization",
	"token",
	"jwttoken",
	"jwt_token",
	"cookie",
	"smscode",
	"sms_code",
	"api_key",
	"apikey",
	"secret",
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

// ShouldLog 检查是否应该记录审计日志（精确匹配优先，避免子串误伤）
func (f *AuditLogFilter) ShouldLog(path string) bool {
	for _, excluded := range f.ExcludedPaths {
		if path == excluded {
			return false
		}
	}
	return true
}

// AuditMiddlewareWithFilter 带过滤器的审计日志中间件（复用全局共享 writer）。
func AuditMiddlewareWithFilter(auditRepo *repository.AuditLogRepository, filter *AuditLogFilter) gin.HandlerFunc {
	inner := AuditMiddleware(auditRepo)
	return func(c *gin.Context) {
		if !filter.ShouldLog(c.Request.URL.Path) {
			c.Next()
			return
		}
		inner(c)
	}
}
