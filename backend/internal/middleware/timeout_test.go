package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestTimeoutMiddlewareExceptKeepsStreamContextAlive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(TimeoutMiddlewareExcept(10*time.Millisecond, "/events", "/ws"))
	router.GET("/events", func(c *gin.Context) {
		select {
		case <-time.After(25 * time.Millisecond):
			c.Status(http.StatusNoContent)
		case <-c.Request.Context().Done():
			c.Status(http.StatusGatewayTimeout)
		}
	})
	router.GET("/api/test", func(c *gin.Context) {
		select {
		case <-time.After(25 * time.Millisecond):
			c.Status(http.StatusNoContent)
		case <-c.Request.Context().Done():
			c.Status(http.StatusGatewayTimeout)
		}
	})

	streamResponse := httptest.NewRecorder()
	router.ServeHTTP(streamResponse, httptest.NewRequest(http.MethodGet, "/events", nil))
	if streamResponse.Code != http.StatusNoContent {
		t.Fatalf("/events status = %d, want %d", streamResponse.Code, http.StatusNoContent)
	}
	if got := streamResponse.Header().Get("X-Request-Timeout"); got != "" {
		t.Fatalf("/events X-Request-Timeout = %q, want empty", got)
	}

	apiResponse := httptest.NewRecorder()
	router.ServeHTTP(apiResponse, httptest.NewRequest(http.MethodGet, "/api/test", nil))
	if apiResponse.Code != http.StatusGatewayTimeout {
		t.Fatalf("ordinary request status = %d, want %d", apiResponse.Code, http.StatusGatewayTimeout)
	}
	if got := apiResponse.Header().Get("X-Request-Timeout"); got != "10ms" {
		t.Fatalf("ordinary X-Request-Timeout = %q, want 10ms", got)
	}
}
