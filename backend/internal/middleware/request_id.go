package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// RequestIDContextKey 是 gin.Context 中保存请求追踪 ID 的键。
	RequestIDContextKey = "request_id"
	RequestIDHeader     = "X-Request-ID"
	TraceIDHeader       = "X-Trace-ID"
)

// RequestIDMiddleware 为每个请求注入稳定的 request id，并同时写回响应头。
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := sanitizeRequestID(c.GetHeader(RequestIDHeader))
		if requestID == "" {
			requestID = sanitizeRequestID(c.GetHeader(TraceIDHeader))
		}
		if requestID == "" {
			requestID = uuid.NewString()
		}

		c.Set(RequestIDContextKey, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Header(TraceIDHeader, requestID)
		c.Next()
	}
}

// GetRequestID 读取当前请求的追踪 ID。
func GetRequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, ok := c.Get(RequestIDContextKey); ok {
		if requestID, ok := value.(string); ok {
			return requestID
		}
	}
	return ""
}

func sanitizeRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if r < 33 || r > 126 {
			return ""
		}
	}
	return value
}
