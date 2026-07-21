package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"caiyun/internal/services"
	appErrors "caiyun/pkg/errors"
	"caiyun/pkg/jwt"
	apiresponse "caiyun/pkg/response"
	"github.com/gin-gonic/gin"
)

const (
	authCookieName    = "auth_token"
	refreshCookieName = "refresh_token"
	csrfCookieName    = "csrf_token"
)

// AuthHandler 认证处理器
type AuthHandler struct {
	authService *services.AuthService
	jwtManager  *jwt.Manager
}

// NewAuthHandler 创建认证处理器
func NewAuthHandler(authService *services.AuthService, jwtManager *jwt.Manager) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		jwtManager:  jwtManager,
	}
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=6"`
	Email    string `json:"email"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type ResetPasswordRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=50"`
	Email       string `json:"email" binding:"required,email"`
	Code        string `json:"code" binding:"required,len=6"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

type SendPasswordResetCodeRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
}

type AuthResponse struct {
	ExpiresAt        int64        `json:"expires_at"`
	RefreshExpiresAt int64        `json:"refresh_expires_at,omitempty"`
	User             UserResponse `json:"user"`
}

type UserResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// Register 用户注册
// @Summary 用户注册
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body services.RegisterRequest true "注册请求"
// @Success 201 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Router /api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req services.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.authService.RegisterContext(c.Request.Context(), &req, requestSessionMetadata(c))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrUserExists):
			respondBusinessError(c, http.StatusConflict, appErrors.BusinessCodeIdentityUsernameExists, "用户名已存在")
		case errors.Is(err, services.ErrEmailExists):
			respondBusinessError(c, http.StatusConflict, appErrors.BusinessCodeIdentityEmailExists, "邮箱已被注册")
		case errors.Is(err, services.ErrWeakPassword):
			respondError(c, http.StatusBadRequest, err.Error())
		default:
			respondInternalServer(c)
		}
		return
	}

	if err := setAuthCookies(c, resp.Token, time.Until(time.Unix(resp.ExpiresAt, 0)), resp.RefreshToken, time.Until(time.Unix(resp.RefreshExpiresAt, 0))); err != nil {
		respondInternalServer(c)
		return
	}
	apiresponse.SuccessCreated(c, AuthResponse{
		ExpiresAt:        resp.ExpiresAt,
		RefreshExpiresAt: resp.RefreshExpiresAt,
		User: UserResponse{
			ID:       resp.User.ID,
			Username: resp.User.Username,
			Email:    resp.User.Email,
			Role:     resp.User.Role,
		},
	})
}

// Login 用户登录
// @Summary 用户登录
// @Tags 认证
// @Accept json
// @Produce json
// @Param request body services.LoginRequest true "登录请求"
// @Success 200 {object} AuthResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req services.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := h.authService.LoginContext(c.Request.Context(), &req, requestSessionMetadata(c))
	if err != nil {
		switch err {
		case services.ErrUserNotFound, services.ErrInvalidCredentials:
			respondBusinessError(c, http.StatusUnauthorized, appErrors.BusinessCodeAuthInvalidCredentials, "用户名或密码错误")
		case services.ErrAccountLocked:
			respondError(c, http.StatusTooManyRequests, "登录失败次数过多，请稍后再试")
		default:
			respondInternalServer(c)
		}
		return
	}

	if err := setAuthCookies(c, resp.Token, time.Until(time.Unix(resp.ExpiresAt, 0)), resp.RefreshToken, time.Until(time.Unix(resp.RefreshExpiresAt, 0))); err != nil {
		respondInternalServer(c)
		return
	}
	apiresponse.Success(c, AuthResponse{
		ExpiresAt:        resp.ExpiresAt,
		RefreshExpiresAt: resp.RefreshExpiresAt,
		User: UserResponse{
			ID:       resp.User.ID,
			Username: resp.User.Username,
			Email:    resp.User.Email,
			Role:     resp.User.Role,
		},
	})
}

// SendPasswordResetCode 发送密码重置邮箱验证码。
func (h *AuthHandler) SendPasswordResetCode(c *gin.Context) {
	var req SendPasswordResetCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.authService.SendPasswordResetCode(req.Username, req.Email); err != nil {
		switch {
		case errors.Is(err, services.ErrEmailServiceDisabled):
			respondError(c, http.StatusServiceUnavailable, "邮箱服务未配置，请联系管理员重置密码")
		case errors.Is(err, services.ErrResetCodeTooFrequent):
			// 与无效用户名/邮箱组合保持相同响应，避免通过冷却状态枚举账号邮箱关联。
			apiresponse.Message(c, "如果用户名和邮箱匹配，验证码将发送到该邮箱")
		default:
			respondInternalServer(c)
		}
		return
	}

	apiresponse.Message(c, "如果用户名和邮箱匹配，验证码将发送到该邮箱")
}

// ResetPassword 通过邮箱验证码重置密码。
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	err := h.authService.ResetPasswordWithCode(req.Username, req.Email, req.Code, req.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidRecoveryInfo), errors.Is(err, services.ErrInvalidResetCode):
			respondError(c, http.StatusBadRequest, "验证码错误或已过期")
		case errors.Is(err, services.ErrWeakPassword):
			respondError(c, http.StatusBadRequest, err.Error())
		default:
			respondInternalServer(c)
		}
		return
	}

	clearAuthCookie(c)
	apiresponse.Message(c, "密码已重置，请使用新密码登录")
}

// RefreshToken rotates an opaque refresh credential. Cookie clients must
// provide the double-submit CSRF token; non-browser clients may send the token
// explicitly in JSON and are not relying on ambient cookie authority.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			respondError(c, http.StatusBadRequest, "请求格式错误")
			return
		}
	}

	bodyToken := strings.TrimSpace(req.RefreshToken)
	cookieToken, cookieErr := c.Cookie(refreshCookieName)
	cookieToken = strings.TrimSpace(cookieToken)
	fromCookie := cookieErr == nil && cookieToken != ""
	if fromCookie && bodyToken != "" && bodyToken != cookieToken {
		clearAuthCookie(c)
		respondError(c, http.StatusUnauthorized, "刷新凭证无效或已过期")
		return
	}
	rawToken := bodyToken
	if rawToken == "" {
		rawToken = cookieToken
	}
	if rawToken == "" {
		clearAuthCookie(c)
		respondError(c, http.StatusUnauthorized, "刷新凭证无效或已过期")
		return
	}
	if fromCookie && !validRefreshCSRF(c) {
		respondError(c, http.StatusForbidden, "CSRF令牌无效")
		return
	}

	resp, err := h.authService.RefreshWithToken(c.Request.Context(), rawToken, requestSessionMetadata(c))
	if err != nil {
		clearAuthCookie(c)
		switch {
		case errors.Is(err, services.ErrInvalidRefreshToken), errors.Is(err, services.ErrRefreshTokenReuse):
			respondError(c, http.StatusUnauthorized, "刷新凭证无效或已过期")
		default:
			respondInternalServer(c)
		}
		return
	}
	if err := setAuthCookies(c, resp.Token, time.Until(time.Unix(resp.ExpiresAt, 0)), resp.RefreshToken, time.Until(time.Unix(resp.RefreshExpiresAt, 0))); err != nil {
		respondInternalServer(c)
		return
	}
	apiresponse.Success(c, TokenResponse{
		ExpiresAt:        resp.ExpiresAt,
		RefreshExpiresAt: resp.RefreshExpiresAt,
	})
}

// Logout revokes the current sid before clearing browser credentials.
func (h *AuthHandler) Logout(c *gin.Context) {
	userID := c.GetUint("user_id")
	sessionID := c.GetString("session_id")
	clearAuthCookie(c)
	if err := h.authService.RevokeSession(c.Request.Context(), userID, sessionID); err != nil {
		respondInternalServer(c)
		return
	}
	apiresponse.Message(c, "退出成功")
}

// LogoutAll revokes every active browser/device session for the current user.
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	userID := c.GetUint("user_id")
	clearAuthCookie(c)
	if err := h.authService.RevokeAllSessions(c.Request.Context(), userID); err != nil {
		respondInternalServer(c)
		return
	}
	apiresponse.Message(c, "所有会话已退出")
}

// GetCurrentUser 获取当前用户信息
// @Summary 获取当前用户信息
// @Tags 认证
// @Accept json
// @Produce json
// @Success 200 {object} UserResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/me [get]
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userID := c.GetUint("user_id")

	user, err := h.authService.GetUserByID(userID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "获取用户信息失败")
		return
	}

	apiresponse.Success(c, UserResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
	})
}

// TokenResponse Token响应
type TokenResponse struct {
	ExpiresAt        int64 `json:"expires_at"`
	RefreshExpiresAt int64 `json:"refresh_expires_at,omitempty"`
}

func setAuthCookies(c *gin.Context, token string, accessMaxAge time.Duration, refreshToken string, refreshMaxAge time.Duration) error {
	if accessMaxAge <= 0 {
		accessMaxAge = 15 * time.Minute
	}
	if refreshMaxAge <= 0 {
		refreshMaxAge = 30 * 24 * time.Hour
	}
	csrfToken, err := generateCSRFToken()
	if err != nil {
		return err
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authCookieName, token, int(accessMaxAge.Seconds()), "/", "", isSecureRequest(c), true)
	if refreshToken != "" {
		c.SetCookie(refreshCookieName, refreshToken, int(refreshMaxAge.Seconds()), "/", "", isSecureRequest(c), true)
	}
	c.SetCookie(csrfCookieName, csrfToken, int(refreshMaxAge.Seconds()), "/", "", isSecureRequest(c), false)
	return nil
}

func clearAuthCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authCookieName, "", -1, "/", "", isSecureRequest(c), true)
	c.SetCookie(refreshCookieName, "", -1, "/", "", isSecureRequest(c), true)
	c.SetCookie(csrfCookieName, "", -1, "/", "", isSecureRequest(c), false)
}

func validRefreshCSRF(c *gin.Context) bool {
	cookieToken, err := c.Cookie(csrfCookieName)
	if err != nil || cookieToken == "" {
		return false
	}
	headerToken := c.GetHeader("X-CSRF-Token")
	if len(cookieToken) != len(headerToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookieToken), []byte(headerToken)) == 1
}

func requestSessionMetadata(c *gin.Context) services.SessionMetadata {
	deviceID := strings.TrimSpace(c.GetHeader("X-Device-ID"))
	userAgent := strings.TrimSpace(c.Request.UserAgent())
	value := userAgent
	if deviceID != "" {
		value = deviceID + " | " + userAgent
	}
	return services.SessionMetadata{DeviceInfo: value, ClientIP: c.ClientIP()}
}

func isSecureRequest(c *gin.Context) bool {
	return c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
}

func generateCSRFToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
