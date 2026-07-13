package middleware

import (
	"log"
	"strconv"
	"time"

	"caiyun/internal/monitor"

	"github.com/gin-gonic/gin"
)

// HTTPMetricsMiddleware 记录 HTTP 请求耗时、状态码，并输出带 request_id 的结构化访问日志。
func HTTPMetricsMiddleware(metrics *monitor.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		duration := time.Since(start)
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		status := strconv.Itoa(c.Writer.Status())
		if metrics != nil {
			metrics.ObserveHTTPRequest(c.Request.Method, route, status, duration.Seconds())
		}
		log.Printf("request_id=%s method=%s path=%s route=%s status=%s duration_ms=%d client_ip=%s user_agent=%q",
			GetRequestID(c),
			c.Request.Method,
			c.Request.URL.Path,
			route,
			status,
			duration.Milliseconds(),
			c.ClientIP(),
			c.Request.UserAgent(),
		)
	}
}
