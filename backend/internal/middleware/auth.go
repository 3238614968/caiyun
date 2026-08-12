package middleware

import (
	"caiyun/internal/repository"
	appErrors "caiyun/pkg/errors"
	"caiyun/pkg/jwt"
	apiresponse "caiyun/pkg/response"
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func abortWithError(c *gin.Context, statusCode int, message string) {
	abortWithBusinessError(c, statusCode, businessCodeForHTTPStatus(statusCode), message)
}

func abortWithBusinessError(c *gin.Context, statusCode int, businessCode appErrors.BusinessCode, message string) {
	if businessCode != "" {
		apiresponse.ErrorWithBusinessCode(c, statusCode, string(businessCode), message)
	} else {
		apiresponse.ErrorWithCode(c, statusCode, message)
	}
	c.Abort()
}

func businessCodeForHTTPStatus(statusCode int) appErrors.BusinessCode {
	switch statusCode {
	case http.StatusUnauthorized:
		return appErrors.BusinessCodeAuthSessionExpired
	case http.StatusForbidden:
		return appErrors.BusinessCodeAuthPermissionDenied
	default:
		return ""
	}
}

func abortWithErrorData(c *gin.Context, statusCode int, message string, data interface{}) {
	apiresponse.ErrorWithData(c, statusCode, message, data)
	c.Abort()
}

// contextKey 是中间件包私有类型，用于 Gin context key，防止外部包覆盖。
type contextKey string

const authFromCookieKey contextKey = "_mw_auth_from_cookie"

type accessSessionStore interface {
	IsActive(ctx context.Context, sessionID string, userID uint, now time.Time) (bool, error)
}

func TimeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return TimeoutMiddlewareExcept(timeout)
}

// TimeoutMiddlewareExcept excludes long-lived upgrade/stream endpoints from the
// normal request deadline. SSE and WebSocket handlers own their connection
// lifetime and must retain the original request context so client disconnects
// remain observable.
func TimeoutMiddlewareExcept(timeout time.Duration, exemptPaths ...string) gin.HandlerFunc {
	exempt := make(map[string]struct{}, len(exemptPaths))
	for _, path := range exemptPaths {
		if path != "" {
			exempt[path] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		if _, ok := exempt[c.Request.URL.Path]; ok {
			c.Next()
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Request-Timeout", timeout.String())
		c.Next()
	}
}

func AuthMiddleware(jwtManager *jwt.Manager) gin.HandlerFunc {
	return AuthMiddlewareWithUser(jwtManager, nil)
}

func AuthMiddlewareWithUser(jwtManager *jwt.Manager, userRepo *repository.UserRepository) gin.HandlerFunc {
	return AuthMiddlewareWithUserAndSession(jwtManager, userRepo, nil)
}

func AuthMiddlewareWithUserAndSession(jwtManager *jwt.Manager, userRepo *repository.UserRepository, sessionStore accessSessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		token := ""
		authFromCookie := false
		cookieToken, cookieErr := c.Cookie("auth_token")
		hasAuthCookie := cookieErr == nil && cookieToken != ""
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				abortWithError(c, http.StatusUnauthorized, "认证格式错误")
				return
			}
			token = parts[1]
			// A bearer token is explicitly supplied by the caller.  Its presence
			// must not turn the request into cookie-authenticated traffic merely
			// because a stale auth cookie is also present.  Otherwise API clients
			// using a bearer token are incorrectly forced through the CSRF branch.
			authFromCookie = false
		} else if hasAuthCookie {
			token = cookieToken
			authFromCookie = true
		}
		if token == "" {
			abortWithError(c, http.StatusUnauthorized, "未提供认证信息")
			return
		}
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			abortWithError(c, http.StatusUnauthorized, "无效的token")
			return
		}

		userID := claims.UserID
		username := claims.Username
		role := claims.Role
		if userRepo != nil {
			user, err := getAuthUserSnapshot(c.Request.Context(), userRepo, claims)
			if err != nil {
				abortWithError(c, http.StatusUnauthorized, "用户不存在或已失效")
				return
			}
			if claims.TokenVersion != user.TokenVersion {
				abortWithBusinessError(c, http.StatusUnauthorized, appErrors.BusinessCodeAuthSessionExpired, "会话已失效，请重新登录")
				return
			}
			username = user.Username
			role = user.Role
		}

		if sessionStore != nil {
			if claims.SessionID == "" {
				abortWithBusinessError(c, http.StatusUnauthorized, appErrors.BusinessCodeAuthSessionExpired, "会话已失效，请重新登录")
				return
			}
			active, err := sessionStore.IsActive(c.Request.Context(), claims.SessionID, claims.UserID, time.Now())
			if err != nil || !active {
				abortWithBusinessError(c, http.StatusUnauthorized, appErrors.BusinessCodeAuthSessionExpired, "会话已失效，请重新登录")
				return
			}
		}

		// 将当前数据库中的用户信息存入上下文，避免角色变更或删号后旧 JWT 继续保留旧权限。
		c.Set("user_id", userID)
		c.Set("username", username)
		c.Set("role", role)
		c.Set("session_id", claims.SessionID)
		c.Set("jwt_id", claims.ID)
		c.Set(string(authFromCookieKey), authFromCookie)

		c.Next()
	}
}

// OptionalAuthMiddlewareWithUser attempts to authenticate a request but never aborts.
// It is intended for endpoints that support either business JWT auth or an alternate
// authentication mechanism, such as API monitor token scraping.
func OptionalAuthMiddlewareWithUser(jwtManager *jwt.Manager, userRepo *repository.UserRepository) gin.HandlerFunc {
	return OptionalAuthMiddlewareWithUserAndSession(jwtManager, userRepo, nil)
}

func OptionalAuthMiddlewareWithUserAndSession(jwtManager *jwt.Manager, userRepo *repository.UserRepository, sessionStore accessSessionStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		token := ""
		authFromCookie := false
		cookieToken, cookieErr := c.Cookie("auth_token")
		hasAuthCookie := cookieErr == nil && cookieToken != ""
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.Next()
				return
			}
			token = parts[1]
			authFromCookie = false
		} else if hasAuthCookie {
			token = cookieToken
			authFromCookie = true
		}
		if token == "" {
			c.Next()
			return
		}
		claims, err := jwtManager.ValidateToken(token)
		if err != nil {
			c.Next()
			return
		}

		userID := claims.UserID
		username := claims.Username
		role := claims.Role
		if userRepo != nil {
			user, err := getAuthUserSnapshot(c.Request.Context(), userRepo, claims)
			if err != nil || claims.TokenVersion != user.TokenVersion {
				c.Next()
				return
			}
			username = user.Username
			role = user.Role
		}

		if sessionStore != nil {
			if claims.SessionID == "" {
				c.Next()
				return
			}
			active, err := sessionStore.IsActive(c.Request.Context(), claims.SessionID, claims.UserID, time.Now())
			if err != nil || !active {
				c.Next()
				return
			}
		}

		c.Set("user_id", userID)
		c.Set("username", username)
		c.Set("role", role)
		c.Set("session_id", claims.SessionID)
		c.Set("jwt_id", claims.ID)
		c.Set(string(authFromCookieKey), authFromCookie)
		c.Next()
	}
}

func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !isUnsafeMethod(c.Request.Method) {
			c.Next()
			return
		}
		if fromCookie, _ := c.Get(string(authFromCookieKey)); fromCookie != true {
			c.Next()
			return
		}

		csrfCookie, err := c.Cookie("csrf_token")
		if err != nil || csrfCookie == "" {
			abortWithError(c, http.StatusForbidden, "缺少CSRF令牌")
			return
		}
		csrfHeader := c.GetHeader("X-CSRF-Token")
		if csrfHeader == "" || subtle.ConstantTimeCompare([]byte(csrfHeader), []byte(csrfCookie)) != 1 {
			abortWithError(c, http.StatusForbidden, "无效的CSRF令牌")
			return
		}

		c.Next()
	}
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role := c.GetString("role")
		if role != "admin" {
			abortWithBusinessError(c, http.StatusForbidden, appErrors.BusinessCodeAuthPermissionDenied, "需要管理员权限")
			return
		}
		c.Next()
	}
}
