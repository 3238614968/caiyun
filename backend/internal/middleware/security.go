package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// BodySizeLimitMiddleware 限制请求体大小，避免未认证接口被超大 body 拖垮。
func BodySizeLimitMiddleware(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil && maxBytes > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

// RecoveryWithLogger 替代默认 gin.Recovery()，把 panic 堆栈通过标准库 log 输出，
// 这样会被 bootstrap.ConfigureStandardLogger 路由到统一的结构化日志。
// 同时向客户端返回 500 通用消息，避免堆栈泄漏。
func RecoveryWithLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				log.Printf("[PANIC] %s %s panic: %v\n%s",
					c.Request.Method, c.Request.URL.Path, rec, stack)
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"code":    http.StatusInternalServerError,
						"message": "服务器内部错误，请稍后再试",
					})
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}
