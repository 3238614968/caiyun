package middleware

import (
	"bytes"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	auditWorkerCount        = 4
	auditBufferSize         = 4096
	auditCaptureSize        = 64 << 10
	auditPersistTimeout     = 3 * time.Second
	auditShutdownDrainTime  = 15 * time.Second
	auditShutdownCancelWait = time.Second
	// A short bounded wait absorbs brief writer bursts without extending normal
	// request latency by the full persistence timeout when storage is degraded.
	auditEnqueueTimeout = 100 * time.Millisecond
)

// asyncAuditWriter 是应用级单例，所有审计中间件共享同一组 worker，
// 避免每个请求都新建 worker goroutine 造成泄漏。
type auditDroppedMetrics interface {
	IncAuditDropped()
}

type auditDroppedMetricsHolder struct {
	metrics auditDroppedMetrics
}

type asyncAuditWriter struct {
	repo     *repository.AuditLogRepository
	ch       chan *models.AuditLog
	stopOnce sync.Once
	mu       sync.RWMutex
	closed   bool
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

var (
	globalAuditWriter        *asyncAuditWriter
	globalAuditWriterMu      sync.Mutex
	globalAuditDroppedMetric atomic.Pointer[auditDroppedMetricsHolder]
	auditDroppedTotal        atomic.Int64
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

// SetAuditDroppedMetrics 注册审计丢弃指标写入器，使队列满时可实时反映到 Prometheus。
func SetAuditDroppedMetrics(metrics auditDroppedMetrics) {
	if metrics == nil {
		globalAuditDroppedMetric.Store(nil)
		return
	}
	globalAuditDroppedMetric.Store(&auditDroppedMetricsHolder{metrics: metrics})
}

func newAsyncAuditWriter(repo *repository.AuditLogRepository) *asyncAuditWriter {
	ctx, cancel := context.WithCancel(context.Background())
	w := &asyncAuditWriter{
		repo:   repo,
		ch:     make(chan *models.AuditLog, auditBufferSize),
		ctx:    ctx,
		cancel: cancel,
	}
	w.wg.Add(auditWorkerCount)
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
		w.mu.Lock()
		w.closed = true
		close(w.ch)
		w.mu.Unlock()

		drained := make(chan struct{})
		go func() {
			w.wg.Wait()
			close(drained)
		}()
		timer := time.NewTimer(auditShutdownDrainTime)
		defer timer.Stop()
		select {
		case <-drained:
			w.cancel()
		case <-timer.C:
			// Bound shutdown even when the database is unhealthy. Cancelling the
			// writer context interrupts in-flight GORM operations and causes
			// workers to discard the remaining buffered entries.
			w.cancel()
			select {
			case <-drained:
			case <-time.After(auditShutdownCancelWait):
			}
		}
	})
}

func (w *asyncAuditWriter) worker() {
	defer w.wg.Done()
	for {
		if w.ctx.Err() != nil {
			return
		}
		select {
		case <-w.ctx.Done():
			return
		case auditLog, ok := <-w.ch:
			if !ok {
				return
			}
			if auditLog == nil || w.repo == nil {
				continue
			}
			persistCtx, cancel := context.WithTimeout(w.ctx, auditPersistTimeout)
			err := w.repo.WithContext(persistCtx).Create(auditLog)
			cancel()
			if err != nil {
				gin.DefaultErrorWriter.Write([]byte("保存审计日志失败: " + err.Error() + "\n"))
			}
		}
	}
}

func (w *asyncAuditWriter) enqueue(auditLog *models.AuditLog) {
	if w == nil || auditLog == nil {
		return
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.closed {
		recordAuditDrop("审计日志 writer 已关闭，丢弃当前审计日志\n")
		return
	}
	select {
	case w.ch <- auditLog:
	case <-time.After(auditEnqueueTimeout):
		recordAuditDrop("审计日志队列已满，丢弃当前审计日志\n")
	}
}

func recordAuditDrop(message string) {
	auditDroppedTotal.Add(1)
	if holder := globalAuditDroppedMetric.Load(); holder != nil && holder.metrics != nil {
		holder.metrics.IncAuditDropped()
	}
	gin.DefaultErrorWriter.Write([]byte(message))
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

		// 通过有界 Tee 捕获请求体，避免为审计而完整复制大请求。
		captureBody := shouldCaptureAuditBody(c)
		requestCapture := newLimitedAuditCapture(auditCaptureSize)
		if c.Request.Body != nil && captureBody {
			originalBody := c.Request.Body
			c.Request.Body = &auditReadCloser{
				Reader: io.TeeReader(originalBody, requestCapture),
				Closer: originalBody,
			}
		}

		// 响应体同样只保留前 auditCaptureSize 字节，实际响应仍完整写给客户端。
		responseCapture := newLimitedAuditCapture(auditCaptureSize)
		blw := &bodyLogWriter{
			ResponseWriter: c.Writer,
			capture:        responseCapture,
			captureBody:    captureBody,
		}
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
			RequestID:    GetRequestID(c),
			IP:           c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
			RequestData:  formatAuditCapture(requestCapture),
			ResponseData: formatAuditCapture(responseCapture),
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

type auditReadCloser struct {
	io.Reader
	io.Closer
}

type limitedAuditCapture struct {
	body      bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedAuditCapture(limit int) *limitedAuditCapture {
	return &limitedAuditCapture{limit: limit}
}

func (c *limitedAuditCapture) Write(p []byte) (int, error) {
	if c == nil {
		return len(p), nil
	}
	remaining := c.limit - c.body.Len()
	if remaining > 0 {
		toWrite := len(p)
		if toWrite > remaining {
			toWrite = remaining
		}
		_, _ = c.body.Write(p[:toWrite])
	}
	if len(p) > remaining {
		c.truncated = true
	}
	return len(p), nil
}

func formatAuditCapture(capture *limitedAuditCapture) string {
	if capture == nil || capture.body.Len() == 0 {
		return ""
	}
	value := truncateString(redactAuditPayload(capture.body.Bytes()), 2000)
	if capture.truncated {
		value += " [capture_truncated]"
	}
	return value
}

func shouldCaptureAuditBody(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	path := strings.ToLower(c.Request.URL.Path)
	if path == "/ws" || strings.Contains(path, "/export") || strings.Contains(path, "/download") {
		return false
	}
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.Contains(contentType, "multipart/form-data") || strings.Contains(contentType, "application/octet-stream") {
		return false
	}
	return !strings.EqualFold(c.GetHeader("Upgrade"), "websocket")
}

// bodyLogWriter 用于有界捕获响应体，不改变真实响应的写入行为。
type bodyLogWriter struct {
	gin.ResponseWriter
	capture     *limitedAuditCapture
	captureBody bool
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	if w.captureBody && w.capture != nil && !isDownloadResponse(w.Header()) {
		_, _ = w.capture.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func (w *bodyLogWriter) WriteString(value string) (int, error) {
	if w.captureBody && w.capture != nil && !isDownloadResponse(w.Header()) {
		_, _ = w.capture.Write([]byte(value))
	}
	return w.ResponseWriter.WriteString(value)
}

func isDownloadResponse(header map[string][]string) bool {
	contentDisposition := strings.ToLower(strings.Join(header["Content-Disposition"], ","))
	contentType := strings.ToLower(strings.Join(header["Content-Type"], ","))
	return strings.Contains(contentDisposition, "attachment") || strings.Contains(contentType, "application/octet-stream")
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
	if maxLen <= 0 {
		return ""
	}
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= maxLen {
		return s
	}
	end := maxLen
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "..."
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
			"/api/v1/auth/refresh",
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
		if filter != nil && !filter.ShouldLog(c.Request.URL.Path) {
			c.Next()
			return
		}
		inner(c)
	}
}
