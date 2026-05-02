package middleware

import (
	"net/http"

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
