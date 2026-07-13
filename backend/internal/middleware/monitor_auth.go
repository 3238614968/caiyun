package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"caiyun/internal/envutil"
	appErrors "caiyun/pkg/errors"

	"github.com/gin-gonic/gin"
)

// MonitorOrAdminMiddleware allows Prometheus-style scraping with API_MONITOR_TOKEN
// while keeping browser/admin access compatible with the existing JWT flow.
func MonitorOrAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if validateMonitorToken(c, envutil.String("API_MONITOR_TOKEN", "")) {
			c.Set("monitor_token_auth", true)
			c.Next()
			return
		}
		role := c.GetString("role")
		if role != "admin" {
			abortWithBusinessError(c, http.StatusForbidden, appErrors.BusinessCodeAuthPermissionDenied, "需要管理员权限或有效监控令牌")
			return
		}
		c.Next()
	}
}

func validateMonitorToken(c *gin.Context, expected string) bool {
	if expected == "" {
		return false
	}
	provided := c.GetHeader("X-Monitor-Token")
	if provided == "" {
		authorization := c.GetHeader("Authorization")
		if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
			provided = strings.TrimSpace(authorization[len("Bearer "):])
		}
	}
	if provided == "" || len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
