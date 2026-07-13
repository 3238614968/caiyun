package middleware

import (
	"context"
	"errors"

	"caiyun/internal/monitor"
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

type captureRateLimitStore struct {
	key    string
	limit  int
	window time.Duration
}

func (s *captureRateLimitStore) RateLimitCheck(key string, limit int, window time.Duration) (bool, int64, time.Duration, error) {
	s.key = key
	s.limit = limit
	s.window = window
	return true, 1, window, nil
}

type failingRateLimitStore struct{}

func (failingRateLimitStore) RateLimitCheck(string, int, time.Duration) (bool, int64, time.Duration, error) {
	return false, 0, 0, errors.New("redis unavailable")
}

func TestRateLimitRedisFailureIsFailClosedWhenConfigured(t *testing.T) {
	config := &RateLimitConfig{
		GlobalRate: 10, GlobalBurst: 10, Backend: "redis",
		RedisWindow: time.Second, FailClosed: true, APIRates: map[string]APIRateLimit{},
	}
	mw := NewAdvancedRateLimitMiddleware(config)
	defer mw.Stop()
	mw.SetRedisStore(failingRateLimitStore{})
	if mw.allow("global:ip:127.0.0.1", 10, mw.globalLimiter) {
		t.Fatal("Redis failure must reject when fail-closed is enabled")
	}
}

func TestRateLimitMissingRedisStoreIsFailClosedWhenConfigured(t *testing.T) {
	config := &RateLimitConfig{
		GlobalRate: 10, GlobalBurst: 10, Backend: "redis",
		RedisWindow: time.Second, FailClosed: true, APIRates: map[string]APIRateLimit{},
	}
	mw := NewAdvancedRateLimitMiddleware(config)
	defer mw.Stop()
	if mw.allow("global:ip:127.0.0.1", 10, mw.globalLimiter) {
		t.Fatal("missing Redis store must reject when fail-closed is enabled")
	}
}

func TestValidateRateLimitConfigRequiresRedisInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	config := &RateLimitConfig{Backend: "memory", RedisWindow: time.Second}
	if err := ValidateRateLimitConfig(config); err == nil {
		t.Fatal("production memory limiter should be rejected")
	}
	t.Setenv("RATE_LIMIT_BACKEND", "")
	if got := DefaultRateLimitConfig().Backend; got != "redis" {
		t.Fatalf("production default backend = %q, want redis", got)
	}
}

func TestValidateRateLimitConfigRejectsUnknownBackend(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	config := &RateLimitConfig{Backend: "typo", RedisWindow: time.Second}
	if err := ValidateRateLimitConfig(config); err == nil {
		t.Fatal("unknown limiter backend should be rejected")
	}
}

func TestRedisFixedWindowLimitUsesRateAndWindow(t *testing.T) {
	if got := redisFixedWindowLimit(5, time.Second); got != 5 {
		t.Fatalf("one second limit=%d, want 5", got)
	}
	if got := redisFixedWindowLimit(5, 2500*time.Millisecond); got != 15 {
		t.Fatalf("ceil multi-second limit=%d, want 15", got)
	}
}

func TestAuthenticatedRateLimitUsesGinRoutePatternForRedis(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &captureRateLimitStore{}
	config := &RateLimitConfig{
		UserRate:    30,
		UserBurst:   50,
		Backend:     "redis",
		RedisWindow: time.Second,
		APIRates: map[string]APIRateLimit{
			"/api/exchange/tasks/:id/execute": {Rate: 2, Burst: 10, ByUser: true},
		},
	}
	mw := NewAuthenticatedRateLimitMiddleware(config)
	mw.SetRedisStore(store)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	})
	router.Use(mw.HandlerFunc())
	router.POST("/api/exchange/tasks/:id/execute", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/exchange/tasks/123/execute", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", w.Code)
	}
	if store.limit != 2 {
		t.Fatalf("redis limit=%d, want API rate 2", store.limit)
	}
	if want := "rate_limit:api:user_1_/api/exchange/tasks/:id/execute"; store.key != want {
		t.Fatalf("redis key=%q, want %q", store.key, want)
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

func TestCORSRejectsDisallowedOriginBeforeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ALLOWED_ORIGINS", "https://allowed.example.com")

	router := gin.New()
	called := false
	router.Use(CORSMiddleware())
	router.POST("/api/test", func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/test", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
	if called {
		t.Fatal("handler should not execute for disallowed origin")
	}
}

func TestAuthenticatedRateLimitRecordsPrometheusRejections(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &RateLimitConfig{
		UserRate:  1,
		UserBurst: 1,
		APIRates:  map[string]APIRateLimit{},
	}
	metrics := monitor.NewMetrics()
	mw := NewAuthenticatedRateLimitMiddleware(config)
	mw.SetMetrics(metrics)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	})
	router.Use(mw.HandlerFunc())
	router.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	first := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	firstW := httptest.NewRecorder()
	router.ServeHTTP(firstW, first)
	if firstW.Code != http.StatusOK {
		t.Fatalf("first status=%d, want 200", firstW.Code)
	}

	second := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	secondW := httptest.NewRecorder()
	router.ServeHTTP(secondW, second)
	if secondW.Code != http.StatusTooManyRequests {
		t.Fatalf("second status=%d, want 429", secondW.Code)
	}

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "caiyun_security_rate_limit_rejected_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["route"] == "/api/test" && labels["reason"] == "USER_RATE_LIMIT" && labels["dimension"] == "user" {
				if got := metric.GetCounter().GetValue(); got != 1 {
					t.Fatalf("counter=%v, want 1", got)
				}
				return
			}
		}
	}
	t.Fatal("caiyun_security_rate_limit_rejected_total label set not found")
}

type fixedAccessSessionStore struct {
	active        bool
	seenSessionID string
	seenUserID    uint
}

func (s *fixedAccessSessionStore) IsActive(_ context.Context, sessionID string, userID uint, _ time.Time) (bool, error) {
	s.seenSessionID = sessionID
	s.seenUserID = userID
	return s.active, nil
}

func TestAuthMiddlewareRequiresActiveServerSessionWhenConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := jwt.NewManager("test-secret-at-least-16-bytes")
	token, err := manager.GenerateAccessToken(7, "alice", "user", 0, "session-7", time.Hour)
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	for _, tc := range []struct {
		name       string
		active     bool
		wantStatus int
	}{
		{name: "active", active: true, wantStatus: http.StatusOK},
		{name: "revoked", active: false, wantStatus: http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fixedAccessSessionStore{active: tc.active}
			router := gin.New()
			router.Use(AuthMiddlewareWithUserAndSession(manager, nil, store))
			router.GET("/protected", func(c *gin.Context) {
				if c.GetString("session_id") != "session-7" || c.GetString("jwt_id") == "" {
					c.Status(http.StatusInternalServerError)
					return
				}
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d", w.Code, tc.wantStatus)
			}
			if store.seenSessionID != "session-7" || store.seenUserID != 7 {
				t.Fatalf("unexpected session lookup sid=%q user=%d", store.seenSessionID, store.seenUserID)
			}
		})
	}
}

func TestAuthMiddlewareRejectsLegacyTokenWithoutSIDWhenSessionStoreConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := jwt.NewManager("test-secret-at-least-16-bytes")
	token, err := manager.GenerateToken(7, "alice", "user", 0, time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken() error = %v", err)
	}
	store := &fixedAccessSessionStore{active: true}
	router := gin.New()
	router.Use(AuthMiddlewareWithUserAndSession(manager, nil, store))
	router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
	if store.seenSessionID != "" {
		t.Fatal("session store should not be queried for missing sid")
	}
}
