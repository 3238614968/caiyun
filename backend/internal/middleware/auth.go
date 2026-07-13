package middleware

import (
	"caiyun/internal/constants"
	"caiyun/internal/envutil"
	"caiyun/internal/monitor"
	"caiyun/internal/repository"
	"caiyun/internal/security/authcache"
	appErrors "caiyun/pkg/errors"
	"caiyun/pkg/jwt"
	apiresponse "caiyun/pkg/response"
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

func abortWithError(c *gin.Context, statusCode int, message string) {
	abortWithBusinessError(c, statusCode, businessCodeForHTTPStatus(statusCode), message)
}

func abortWithBusinessError(c *gin.Context, statusCode int, businessCode appErrors.BusinessCode, message string) {
	if businessCode != "" {
		apiresponse.ErrorWithBusinessCode(c, statusCode, string(businessCode), message)
	} else {
		apiresponse.ErrorWithCode(c, statusCode, message)
	}
	c.Abort()
}

func businessCodeForHTTPStatus(statusCode int) appErrors.BusinessCode {
	switch statusCode {
	case http.StatusUnauthorized:
		return appErrors.BusinessCodeAuthSessionExpired
	case http.StatusForbidden:
		return appErrors.BusinessCodeAuthPermissionDenied
	default:
		return ""
	}
}

func abortWithErrorData(c *gin.Context, statusCode int, message string, data interface{}) {
	apiresponse.ErrorWithData(c, statusCode, message, data)
	c.Abort()
}

// contextKey 是中间件包私有类型，用于 Gin context key，防止外部包覆盖。
type contextKey string

const authFromCookieKey contextKey = "_mw_auth_from_cookie"

// RateLimiter 基于令牌桶的限流器
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // 每秒允许的请求数
	burst    int           // 突发容量
	cleanup  time.Duration // 清理间隔
	stopCh   chan struct{}
	stopOnce sync.Once
}

type visitor struct {
	tokens    float64
	lastSeen  time.Time
	maxTokens float64
	rate      float64
}

type authUserSnapshot = authcache.Snapshot

type accessSessionStore interface {
	IsActive(ctx context.Context, sessionID string, userID uint, now time.Time) (bool, error)
}

type rateLimitStore interface {
	RateLimitCheck(key string, limit int, window time.Duration) (bool, int64, time.Duration, error)
}

func NewRateLimiter(rate, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		burst:    burst,
		cleanup:  5 * time.Minute,
		stopCh:   make(chan struct{}),
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			for ip, v := range rl.visitors {
				if time.Since(v.lastSeen) > rl.cleanup {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) Stop() {
	if rl == nil {
		return
	}
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[key]
	now := time.Now()

	if !exists {
		rl.visitors[key] = &visitor{
			tokens:    float64(rl.burst) - 1,
			lastSeen:  now,
			maxTokens: float64(rl.burst),
			rate:      float64(rl.rate),
		}
		return true
	}

	// 补充令牌
	elapsed := now.Sub(v.lastSeen).Seconds()
	v.tokens += elapsed * v.rate
	if v.tokens > v.maxTokens {
		v.tokens = v.maxTokens
	}
	v.lastSeen = now

	if v.tokens >= 1 {
		v.tokens--
		return true
	}
	return false
}

// RateLimitMiddleware 限流中间件
func RateLimitMiddleware(rate, burst int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, burst)
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !limiter.Allow(key) {
			abortWithError(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}
		c.Next()
	}
}

// RateLimitByUserMiddleware 基于用户的限流中间件（用于需要登录的API）
func RateLimitByUserMiddleware(rate, burst int) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, burst)
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			abortWithError(c, http.StatusUnauthorized, "未授权")
			return
		}

		key := fmt.Sprintf("user_%d", userID.(uint))
		if !limiter.Allow(key) {
			abortWithError(c, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
			return
		}
		c.Next()
	}
}

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	// 全局限流：每个IP的请求限制
	GlobalRate  int
	GlobalBurst int

	// 用户限流：每个用户的请求限制
	UserRate  int
	UserBurst int

	// 特定API限流配置
	APIRates map[string]APIRateLimit

	// Backend 支持 memory / redis；redis 用固定窗口计数，适合多副本部署。
	Backend     string
	RedisWindow time.Duration
	// FailClosed rejects requests when the shared limiter is unavailable. It
	// is mandatory in production so replicas never silently diverge to local
	// memory buckets during a Redis outage.
	FailClosed bool
}

// APIRateLimit 特定API的限流配置
type APIRateLimit struct {
	Rate   int
	Burst  int
	ByUser bool // 是否基于用户限流，false则基于IP
}

// DefaultRateLimitConfig 默认限流配置（使用常量）
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		GlobalRate:  constants.DefaultGlobalRate,  // 每秒100请求
		GlobalBurst: constants.DefaultGlobalBurst, // 突发150请求
		UserRate:    constants.DefaultUserRate,    // 每秒30请求
		UserBurst:   constants.DefaultUserBurst,   // 突发50请求
		Backend:     rateLimitBackendFromEnv(),
		RedisWindow: rateLimitRedisWindowFromEnv(),
		FailClosed:  isProductionEnvironment(),
		APIRates: map[string]APIRateLimit{
			// 兑换API：更严格的限流
			"/api/exchange/tasks":               {Rate: constants.ExchangeTaskRate, Burst: constants.ExchangeTaskBurst, ByUser: true},
			"/api/exchange/tasks/:id/execute":   {Rate: constants.ExchangeTaskRate, Burst: constants.ExchangeTaskBurst, ByUser: true},
			"/api/exchange/tasks/batch-execute": {Rate: constants.BatchExecuteRate, Burst: constants.BatchExecuteBurst, ByUser: true},
			"/api/exchange/immediate":           {Rate: constants.ExchangeTaskRate, Burst: constants.ExchangeTaskBurst, ByUser: true},
			"/api/exchange/records/export":      {Rate: constants.ExportRate, Burst: constants.ExportBurst, ByUser: true},
			// 登录API：防止暴力破解
			"/api/auth/login":                    {Rate: 5, Burst: 10, ByUser: false},
			"/api/auth/register":                 {Rate: 3, Burst: 5, ByUser: false},
			"/api/auth/password/reset-code/send": {Rate: 1, Burst: 3, ByUser: false},
			"/api/auth/password/reset":           {Rate: 3, Burst: 5, ByUser: false},
			// 商品搜索API
			"/api/products/search": {Rate: 20, Burst: 30, ByUser: true},
		},
	}
}

func rateLimitBackendFromEnv() string {
	defaultBackend := "memory"
	if isProductionEnvironment() {
		defaultBackend = "redis"
	}
	return strings.ToLower(strings.TrimSpace(envutil.String("RATE_LIMIT_BACKEND", defaultBackend)))
}

func isProductionEnvironment() bool {
	return strings.EqualFold(strings.TrimSpace(envutil.String("APP_ENV", "development")), "production")
}

// ValidateRateLimitConfig rejects ambiguous or unsafe backends at startup.
// Production always requires the shared Redis limiter; memory remains an
// explicit local-development option only.
func ValidateRateLimitConfig(config *RateLimitConfig) error {
	if config == nil {
		return fmt.Errorf("限流配置不能为空")
	}
	switch config.Backend {
	case "memory":
		if isProductionEnvironment() {
			return fmt.Errorf("生产环境 RATE_LIMIT_BACKEND 必须为 redis")
		}
	case "redis":
	default:
		return fmt.Errorf("不支持的 RATE_LIMIT_BACKEND=%q（仅支持 memory/redis）", config.Backend)
	}
	if config.RedisWindow <= 0 {
		return fmt.Errorf("RATE_LIMIT_REDIS_WINDOW 必须大于 0")
	}
	return nil
}

func rateLimitRedisWindowFromEnv() time.Duration {
	return envutil.Duration("RATE_LIMIT_REDIS_WINDOW", time.Second)
}

// AdvancedRateLimitMiddleware 高级限流中间件（认证前使用）。
// 认证前只执行全局 IP 限流和显式 IP 维度接口限流；需要用户维度的接口由
// AuthenticatedRateLimitMiddleware 在认证后处理，避免 user_id 尚未写入上下文时退化为 IP 限流。
func AdvancedRateLimitMiddleware(config *RateLimitConfig) gin.HandlerFunc {
	mw := NewAdvancedRateLimitMiddleware(config)
	return mw.HandlerFunc()
}

// RateLimitMiddlewareInstance 包装限流器与清理协程，便于优雅退出时 Stop。
type RateLimitMiddlewareInstance struct {
	globalLimiter *RateLimiter
	apiLimiters   map[string]*RateLimiter
	config        *RateLimitConfig
	byUser        bool
	store         rateLimitStore
	backend       string
	redisWindow   time.Duration
	metrics       *monitor.Metrics
}

// NewAdvancedRateLimitMiddleware 创建认证前的全局限流中间件实例。
func NewAdvancedRateLimitMiddleware(config *RateLimitConfig) *RateLimitMiddlewareInstance {
	mw := &RateLimitMiddlewareInstance{
		globalLimiter: NewRateLimiter(config.GlobalRate, config.GlobalBurst),
		apiLimiters:   make(map[string]*RateLimiter),
		config:        config,
		byUser:        false,
		backend:       config.Backend,
		redisWindow:   config.RedisWindow,
	}
	for path, rateConfig := range config.APIRates {
		if !rateConfig.ByUser {
			mw.apiLimiters[path] = NewRateLimiter(rateConfig.Rate, rateConfig.Burst)
		}
	}
	return mw
}

// NewAuthenticatedRateLimitMiddleware 创建认证后的用户维度限流中间件实例。
func NewAuthenticatedRateLimitMiddleware(config *RateLimitConfig) *RateLimitMiddlewareInstance {
	mw := &RateLimitMiddlewareInstance{
		globalLimiter: NewRateLimiter(config.UserRate, config.UserBurst), // 复用为 user 维度默认限流器
		apiLimiters:   make(map[string]*RateLimiter),
		config:        config,
		byUser:        true,
		backend:       config.Backend,
		redisWindow:   config.RedisWindow,
	}
	for path, rateConfig := range config.APIRates {
		if rateConfig.ByUser {
			mw.apiLimiters[path] = NewRateLimiter(rateConfig.Rate, rateConfig.Burst)
		}
	}
	return mw
}

// SetRedisStore 启用 Redis 限流后端。生产配置下 Redis 缺失或调用
// 失败会拒绝请求，不允许静默降级为进程内限流。
func (m *RateLimitMiddlewareInstance) SetRedisStore(store rateLimitStore) {
	if m == nil {
		return
	}
	m.store = store
}

// SetMetrics 绑定 Prometheus 指标收集器，用于记录限流拒绝事件。
func (m *RateLimitMiddlewareInstance) SetMetrics(metrics *monitor.Metrics) {
	if m == nil {
		return
	}
	m.metrics = metrics
}

func (m *RateLimitMiddlewareInstance) allow(key string, rate int, fallback *RateLimiter) bool {
	if rate <= 0 {
		rate = 1
	}
	if m != nil && m.backend == "redis" {
		if m.store == nil {
			log.Printf("[RateLimit] Redis 限流存储未配置 key=%s fail_closed=%t", key, m.config.FailClosed)
			if m.config.FailClosed {
				return false
			}
		} else {
			window := m.redisWindow
			if window <= 0 {
				window = time.Second
			}
			limit := redisFixedWindowLimit(rate, window)
			allowed, _, _, err := m.store.RateLimitCheck("rate_limit:"+key, limit, window)
			if err == nil {
				return allowed
			}
			log.Printf("[RateLimit] Redis 限流失败 key=%s fail_closed=%t err=%v", key, m.config.FailClosed, err)
			if m.config.FailClosed {
				return false
			}
		}
	}
	return fallback.Allow(key)
}

func redisFixedWindowLimit(rate int, window time.Duration) int {
	if rate <= 0 {
		rate = 1
	}
	if window <= 0 {
		return rate
	}
	seconds := int(window / time.Second)
	if window%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		seconds = 1
	}
	return rate * seconds
}

func rateLimitRoutePath(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	path := c.Request.URL.Path
	if fullPath := c.FullPath(); fullPath != "" {
		path = fullPath
	}
	return normalizeRateLimitRoutePath(path)
}

func normalizeRateLimitRoutePath(path string) string {
	if strings.HasPrefix(path, "/api/v1/") {
		return "/api/" + strings.TrimPrefix(path, "/api/v1/")
	}
	if path == "/api/v1" {
		return "/api"
	}
	return path
}

func (m *RateLimitMiddlewareInstance) recordRejection(route, reason, dimension string) {
	if m == nil || m.metrics == nil {
		return
	}
	m.metrics.RecordRateLimitRejection(route, reason, dimension)
}

// HandlerFunc 返回 gin 中间件函数。
func (m *RateLimitMiddlewareInstance) HandlerFunc() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := rateLimitRoutePath(c)

		if !m.byUser {
			// 认证前：全局 IP 限流
			clientIP := c.ClientIP()
			if !m.allow("global:ip:"+clientIP, m.config.GlobalRate, m.globalLimiter) {
				m.recordRejection(path, "GLOBAL_RATE_LIMIT", "ip")
				abortWithErrorData(c, http.StatusTooManyRequests, "服务器繁忙，请稍后再试", gin.H{
					"reason": "GLOBAL_RATE_LIMIT",
				})
				return
			}
			if apiLimit, exists := m.config.APIRates[path]; exists && !apiLimit.ByUser {
				limiter := m.apiLimiters[path]
				key := fmt.Sprintf("ip_%s_%s", clientIP, path)
				if !m.allow("api:"+key, apiLimit.Rate, limiter) {
					m.recordRejection(path, "API_RATE_LIMIT", "ip")
					abortWithErrorData(c, http.StatusTooManyRequests, "该接口请求过于频繁，请稍后再试", gin.H{
						"reason": "API_RATE_LIMIT",
						"path":   path,
					})
					return
				}
			}
			c.Next()
			return
		}

		// 认证后：用户维度限流
		userID, exists := c.Get("user_id")
		if !exists {
			abortWithError(c, http.StatusUnauthorized, "未授权")
			return
		}
		if apiLimit, exists := m.config.APIRates[path]; exists && apiLimit.ByUser {
			limiter := m.apiLimiters[path]
			key := fmt.Sprintf("user_%d_%s", userID.(uint), path)
			if !m.allow("api:"+key, apiLimit.Rate, limiter) {
				m.recordRejection(path, "API_RATE_LIMIT", "user")
				abortWithErrorData(c, http.StatusTooManyRequests, "该接口请求过于频繁，请稍后再试", gin.H{
					"reason": "API_RATE_LIMIT",
					"path":   path,
				})
				return
			}
			c.Next()
			return
		}
		key := fmt.Sprintf("user_default_%d", userID.(uint))
		if !m.allow(key, m.config.UserRate, m.globalLimiter) {
			m.recordRejection(path, "USER_RATE_LIMIT", "user")
			abortWithErrorData(c, http.StatusTooManyRequests, "您的请求过于频繁，请稍后再试", gin.H{
				"reason": "USER_RATE_LIMIT",
			})
			return
		}
		c.Next()
	}
}

// Stop 停止所有内部限流器的清理协程。
func (m *RateLimitMiddlewareInstance) Stop() {
	if m == nil {
		return
	}
	if m.globalLimiter != nil {
		m.globalLimiter.Stop()
	}
	for _, limiter := range m.apiLimiters {
		limiter.Stop()
	}
}

// AuthenticatedRateLimitMiddleware 认证后用户维度限流中间件（保留向后兼容签名）。
func AuthenticatedRateLimitMiddleware(config *RateLimitConfig) gin.HandlerFunc {
	mw := NewAuthenticatedRateLimitMiddleware(config)
	return mw.HandlerFunc()
}

// TimeoutMiddleware 请求超时中间件
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-Timeout", timeout.String())
		c.Next()
	}
}

func AuthMiddleware(jwtManager *jwt.Manager) gin.HandlerFunc {
	return AuthMiddlewareWithUser(jwtManager, nil)
}

func AuthMiddlewareWithUser(jwtManager *jwt.Manager, userRepo *repository.UserRepository) gin.HandlerFunc {
	return AuthMiddlewareWithUserAndSession(jwtManager, userRepo, nil)
}

func AuthMiddlewareWithUserAndSession(jwtManager *jwt.Manager, userRepo *repository.UserRepository, sessionStore accessSessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		token := ""
		authFromCookie := false
		cookieToken, cookieErr := c.Cookie("auth_token")
		hasAuthCookie := cookieErr == nil && cookieToken != ""
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				abortWithError(c, http.StatusUnauthorized, "认证格式错误")
				return
			}
			token = parts[1]
			authFromCookie = hasAuthCookie
		} else if hasAuthCookie {
			token = cookieToken
			authFromCookie = true
		}
		if token == "" {
			abortWithError(c, http.StatusUnauthorized, "未提供认证信息")
			return
		}
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			abortWithError(c, http.StatusUnauthorized, "无效的token")
			return
		}

		userID := claims.UserID
		username := claims.Username
		role := claims.Role
		if userRepo != nil {
			user, err := getAuthUserSnapshot(c.Request.Context(), userRepo, claims)
			if err != nil {
				abortWithError(c, http.StatusUnauthorized, "用户不存在或已失效")
				return
			}
			if claims.TokenVersion != user.TokenVersion {
				abortWithBusinessError(c, http.StatusUnauthorized, appErrors.BusinessCodeAuthSessionExpired, "会话已失效，请重新登录")
				return
			}
			username = user.Username
			role = user.Role
		}

		if sessionStore != nil {
			if claims.SessionID == "" {
				abortWithBusinessError(c, http.StatusUnauthorized, appErrors.BusinessCodeAuthSessionExpired, "会话已失效，请重新登录")
				return
			}
			active, err := sessionStore.IsActive(c.Request.Context(), claims.SessionID, claims.UserID, time.Now())
			if err != nil || !active {
				abortWithBusinessError(c, http.StatusUnauthorized, appErrors.BusinessCodeAuthSessionExpired, "会话已失效，请重新登录")
				return
			}
		}

		// 将当前数据库中的用户信息存入上下文，避免角色变更或删号后旧 JWT 继续保留旧权限。
		c.Set("user_id", userID)
		c.Set("username", username)
		c.Set("role", role)
		c.Set("session_id", claims.SessionID)
		c.Set("jwt_id", claims.ID)
		c.Set(string(authFromCookieKey), authFromCookie)

		c.Next()
	}
}

// OptionalAuthMiddlewareWithUser attempts to authenticate a request but never aborts.
// It is intended for endpoints that support either business JWT auth or an alternate
// authentication mechanism, such as API monitor token scraping.
func OptionalAuthMiddlewareWithUser(jwtManager *jwt.Manager, userRepo *repository.UserRepository) gin.HandlerFunc {
	return OptionalAuthMiddlewareWithUserAndSession(jwtManager, userRepo, nil)
}

func OptionalAuthMiddlewareWithUserAndSession(jwtManager *jwt.Manager, userRepo *repository.UserRepository, sessionStore accessSessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		token := ""
		authFromCookie := false
		cookieToken, cookieErr := c.Cookie("auth_token")
		hasAuthCookie := cookieErr == nil && cookieToken != ""
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.Next()
				return
			}
			token = parts[1]
			authFromCookie = hasAuthCookie
		} else if hasAuthCookie {
			token = cookieToken
			authFromCookie = true
		}
		if token == "" {
			c.Next()
			return
		}
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			c.Next()
			return
		}

		userID := claims.UserID
		username := claims.Username
		role := claims.Role
		if userRepo != nil {
			user, err := getAuthUserSnapshot(c.Request.Context(), userRepo, claims)
			if err != nil || claims.TokenVersion != user.TokenVersion {
				c.Next()
				return
			}
			username = user.Username
			role = user.Role
		}

		if sessionStore != nil {
			if claims.SessionID == "" {
				c.Next()
				return
			}
			active, err := sessionStore.IsActive(c.Request.Context(), claims.SessionID, claims.UserID, time.Now())
			if err != nil || !active {
				c.Next()
				return
			}
		}

		c.Set("user_id", userID)
		c.Set("username", username)
		c.Set("role", role)
		c.Set("session_id", claims.SessionID)
		c.Set("jwt_id", claims.ID)
		c.Set(string(authFromCookieKey), authFromCookie)
		c.Next()
	}
}

func getAuthUserSnapshot(ctx context.Context, userRepo *repository.UserRepository, claims *jwt.Claims) (*authUserSnapshot, error) {
	now := time.Now()
	if snapshot, ok := authcache.Load(claims.UserID, claims.TokenVersion, now); ok {
		return snapshot, nil
	}

	user, err := userRepo.WithContext(ctx).FindByID(claims.UserID)
	if err != nil {
		authcache.Delete(claims.UserID)
		return nil, err
	}
	return authcache.Store(user.ID, authcache.Entry{
		Username:     user.Username,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
		ExpiresAt:    now.Add(authUserCacheTTL()),
	}), nil
}

func authUserCacheTTL() time.Duration {
	// 缩短默认 TTL 到 10 秒，降低改密/重置后旧会话仍可用的窗口。
	return envutil.Duration("AUTH_USER_CACHE_TTL", 10*time.Second)
}

// InvalidateAuthUserCache 在用户改密、重置密码、删除等场景主动清除缓存，
// 让 token_version 变更立即生效。
func InvalidateAuthUserCache(userID uint) {
	authcache.Delete(userID)
}

func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isUnsafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if fromCookie, _ := c.Get(string(authFromCookieKey)); fromCookie != true {
			c.Next()
			return
		}

		csrfCookie, err := c.Cookie("csrf_token")
		if err != nil || csrfCookie == "" {
			abortWithError(c, http.StatusForbidden, "缺少CSRF令牌")
			return
		}
		csrfHeader := c.GetHeader("X-CSRF-Token")
		if csrfHeader == "" || subtle.ConstantTimeCompare([]byte(csrfHeader), []byte(csrfCookie)) != 1 {
			abortWithError(c, http.StatusForbidden, "无效的CSRF令牌")
			return
		}

		c.Next()
	}
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" {
			abortWithBusinessError(c, http.StatusForbidden, appErrors.BusinessCodeAuthPermissionDenied, "需要管理员权限")
			return
		}
		c.Next()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if isAllowedOrigin(origin) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
				c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
				c.Writer.Header().Add("Vary", "Origin")
				c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
				c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
				c.Writer.Header().Set("Access-Control-Max-Age", "600")
			} else {
				// Origin 不在白名单：显式返回 403，避免浏览器侧看到 CORS 报错但服务端已执行写操作。
				abortWithError(c, http.StatusForbidden, "跨域来源未被允许")
				return
			}
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func isAllowedOrigin(origin string) bool {
	allowedOrigins := envutil.String("ALLOWED_ORIGINS", "")
	if allowedOrigins == "" {
		return false
	}
	for _, allowed := range strings.Split(allowedOrigins, ",") {
		allowed = strings.TrimSpace(allowed)
		if allowed != "" && allowed == origin {
			return true
		}
	}
	return false
}
