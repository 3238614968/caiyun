package middleware

import (
	"caiyun/internal/constants"
	"caiyun/pkg/jwt"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter 基于令牌桶的限流器
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // 每秒允许的请求数
	burst    int           // 突发容量
	cleanup  time.Duration // 清理间隔
}

type visitor struct {
	tokens    float64
	lastSeen  time.Time
	maxTokens float64
	rate      float64
}

func NewRateLimiter(rate, burst int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		burst:    burst,
		cleanup:  5 * time.Minute,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > rl.cleanup {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
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
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后再试"})
			c.Abort()
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
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
			c.Abort()
			return
		}

		key := fmt.Sprintf("user_%d", userID.(uint))
		if !limiter.Allow(key) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "请求过于频繁，请稍后再试"})
			c.Abort()
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
}

// APIRateLimit 特定API的限流配置
type APIRateLimit struct {
	Rate  int
	Burst int
	ByUser bool // 是否基于用户限流，false则基于IP
}

// DefaultRateLimitConfig 默认限流配置（使用常量）
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		GlobalRate:  constants.DefaultGlobalRate,  // 每秒100请求
		GlobalBurst: constants.DefaultGlobalBurst, // 突发150请求
		UserRate:    constants.DefaultUserRate,    // 每秒30请求
		UserBurst:   constants.DefaultUserBurst,   // 突发50请求
		APIRates: map[string]APIRateLimit{
			// 兑换API：更严格的限流
			"/api/exchange/tasks":               {Rate: constants.ExchangeTaskRate,  Burst: constants.ExchangeTaskBurst,  ByUser: true},
			"/api/exchange/tasks/batch-execute": {Rate: constants.BatchExecuteRate,  Burst: constants.BatchExecuteBurst,  ByUser: true},
			"/api/exchange/records/export":      {Rate: constants.ExportRate,        Burst: constants.ExportBurst,        ByUser: true},
			// 登录API：防止暴力破解
			"/api/auth/login":    {Rate: 5, Burst: 10, ByUser: false},
			"/api/auth/register": {Rate: 3, Burst: 5, ByUser: false},
			// 商品搜索API
			"/api/products/search": {Rate: 20, Burst: 30, ByUser: true},
		},
	}
}

// AdvancedRateLimitMiddleware 高级限流中间件（支持不同API不同限流策略）
func AdvancedRateLimitMiddleware(config *RateLimitConfig) gin.HandlerFunc {
	// 创建多个限流器
	globalLimiter := NewRateLimiter(config.GlobalRate, config.GlobalBurst)
	userLimiter := NewRateLimiter(config.UserRate, config.UserBurst)
	apiLimiters := make(map[string]*RateLimiter)

	for path, rateConfig := range config.APIRates {
		apiLimiters[path] = NewRateLimiter(rateConfig.Rate, rateConfig.Burst)
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// 1. 全局IP限流
		clientIP := c.ClientIP()
		if !globalLimiter.Allow(clientIP) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "服务器繁忙，请稍后再试",
				"code":  "GLOBAL_RATE_LIMIT",
			})
			c.Abort()
			return
		}

		// 2. 特定API限流
		if apiLimit, exists := config.APIRates[path]; exists {
			limiter := apiLimiters[path]
			var key string

			if apiLimit.ByUser {
				// 基于用户限流
				if userID, exists := c.Get("user_id"); exists {
					key = fmt.Sprintf("user_%d_%s", userID.(uint), path)
				} else {
					// 未登录用户使用IP
					key = fmt.Sprintf("ip_%s_%s", clientIP, path)
				}
			} else {
				// 基于IP限流
				key = fmt.Sprintf("ip_%s_%s", clientIP, path)
			}

			if !limiter.Allow(key) {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error": "该接口请求过于频繁，请稍后再试",
					"code":  "API_RATE_LIMIT",
					"path":  path,
				})
				c.Abort()
				return
			}
		} else {
			// 3. 默认用户限流（针对已登录用户）
			if userID, exists := c.Get("user_id"); exists {
				key := fmt.Sprintf("user_default_%d", userID.(uint))
				if !userLimiter.Allow(key) {
					c.JSON(http.StatusTooManyRequests, gin.H{
						"error": "您的请求过于频繁，请稍后再试",
						"code":  "USER_RATE_LIMIT",
					})
					c.Abort()
					return
				}
			}
		}

		c.Next()
	}
}

// TimeoutMiddleware 请求超时中间件
func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(
			c.Request.Context(),
		)
		c.Header("X-Request-Timeout", timeout.String())
		c.Next()
	}
}

func AuthMiddleware(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证信息"})
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "认证格式错误"})
			c.Abort()
			return
		}

		token := parts[1]
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的token"})
			c.Abort()
			return
		}

		// 将用户信息存入上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		c.Next()
	}
}

func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
