package middleware

import (
	"caiyun/internal/envutil"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware applies the explicit browser-origin policy. It deliberately
// rejects an untrusted Origin before the request reaches a write handler.
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
				abortWithError(c, http.StatusForbidden, "跨域来源未被允许")
				return
			}
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
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
