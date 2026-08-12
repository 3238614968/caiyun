package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCORSMiddlewareAllowsConfiguredPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ALLOWED_ORIGINS", "https://console.example.com, https://admin.example.com")

	router := gin.New()
	called := false
	router.Use(CORSMiddleware())
	router.POST("/api/v1/operations", func(c *gin.Context) {
		called = true
		c.Status(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/operations", nil)
	req.Header.Set("Origin", "https://console.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d, want %d", w.Code, http.StatusNoContent)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://console.example.com" {
		t.Fatalf("allow-origin=%q", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow-credentials=%q", got)
	}
	if got := w.Header().Get("Vary"); got != "Origin" {
		t.Fatalf("vary=%q", got)
	}
	if called {
		t.Fatal("preflight must not reach the protected handler")
	}
}
