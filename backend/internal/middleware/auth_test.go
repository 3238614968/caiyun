package middleware

import (
	"caiyun/pkg/jwt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterBurstAndRefill(t *testing.T) {
	limiter := NewRateLimiter(1, 2)
	if !limiter.Allow("user-1") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("user-1") {
		t.Fatal("second request should be allowed by burst")
	}
	if limiter.Allow("user-1") {
		t.Fatal("third request should be rate limited")
	}
}

func TestAuthorizationHeaderWithAuthCookieStillRequiresCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)

	manager := jwt.NewManager("test-secret-at-least-16-bytes")
	token, err := manager.GenerateToken(1, "alice", "user", 0, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}

	router := gin.New()
	router.Use(AuthMiddleware(manager))
	router.Use(CSRFMiddleware())
	router.POST("/api/protected", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "stale-cookie-token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("mixed header+cookie auth without csrf status=%d, want 403", w.Code)
	}
}

func TestAuthenticatedRateLimitUsesUserDimension(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &RateLimitConfig{
		UserRate:  1,
		UserBurst: 1,
		APIRates:  map[string]APIRateLimit{},
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if c.GetHeader("X-Test-User") == "2" {
			c.Set("user_id", uint(2))
		} else {
			c.Set("user_id", uint(1))
		}
		c.Next()
	})
	router.Use(AuthenticatedRateLimitMiddleware(config))
	router.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first user request status=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second same-user request status=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Test-User", "2")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("different user should have independent limit, status=%d", w.Code)
	}
}

func TestCSRFMiddlewareRequiresMatchingTokenForCookieAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(authFromCookieKey), true)
		c.Next()
	})
	router.Use(CSRFMiddleware())
	router.POST("/api/protected", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing csrf header status=%d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/protected", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: "token"})
	req.Header.Set("X-CSRF-Token", "token")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("matching csrf token status=%d", w.Code)
	}
}

func TestCORSDefaultOriginsDisabledInProduction(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("APP_ENV", "production")
	if isAllowedOrigin("http://localhost:5173") {
		t.Fatal("localhost fallback origins must not be allowed in production")
	}
}

func TestCORSRequiresExplicitOriginsOutsideProduction(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("APP_ENV", "development")
	if isAllowedOrigin("http://localhost:5173") {
		t.Fatal("localhost fallback origin should not be allowed without ALLOWED_ORIGINS")
	}
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:5173")
	if !isAllowedOrigin("http://localhost:5173") {
		t.Fatal("explicit localhost origin should be allowed")
	}
}
