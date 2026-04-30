package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	"caiyun/internal/services"
	"caiyun/pkg/jwt"
	"github.com/gin-gonic/gin"
)

const (
	authCookieName = "auth_token"
	csrfCookieName = "csrf_token"
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
	Password string `json:"password" binding:"required,min=12"`
	Email    string `json:"email"`
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type ResetPasswordRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=50"`
	Email       string `json:"email" binding:"required,email"`
	NewPassword string `json:"new_password" binding:"required,min=12"`
}

type AuthResponse struct {
	ExpiresAt int64        `json:"expires_at"`
	User      UserResponse `json:"user"`
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
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	resp, err := h.authService.Register(&req)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrUserExists):
			c.JSON(http.StatusConflict, ErrorResponse{Message: "用户名已存在"})
		case errors.Is(err, services.ErrEmailExists):
			c.JSON(http.StatusConflict, ErrorResponse{Message: "邮箱已被注册"})
		case errors.Is(err, services.ErrWeakPassword):
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		}
		return
	}

	setAuthCookies(c, resp.Token, time.Until(time.Unix(resp.ExpiresAt, 0)))
	c.JSON(http.StatusCreated, AuthResponse{
		ExpiresAt: resp.ExpiresAt,
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
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	resp, err := h.authService.Login(&req)
	if err != nil {
		switch err {
		case services.ErrUserNotFound, services.ErrInvalidCredentials:
			c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "用户名或密码错误"})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		}
		return
	}

	setAuthCookies(c, resp.Token, time.Until(time.Unix(resp.ExpiresAt, 0)))
	c.JSON(http.StatusOK, AuthResponse{
		ExpiresAt: resp.ExpiresAt,
		User: UserResponse{
			ID:       resp.User.ID,
			Username: resp.User.Username,
			Email:    resp.User.Email,
			Role:     resp.User.Role,
		},
	})
}

// ResetPassword 通过用户名和注册邮箱重置密码。
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	err := h.authService.ResetPasswordByEmail(req.Username, req.Email, req.NewPassword)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrInvalidRecoveryInfo):
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: "用户名或邮箱不匹配"})
		case errors.Is(err, services.ErrWeakPassword):
			c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "密码重置失败"})
		}
		return
	}

	clearAuthCookie(c)
	c.JSON(http.StatusOK, SuccessResponse{Message: "密码已重置，请使用新密码登录"})
}

// RefreshToken 刷新Token
// @Summary 刷新Token
// @Tags 认证
// @Accept json
// @Produce json
// @Success 200 {object} TokenResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/auth/refresh [post]
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	userID := c.GetUint("user_id")

	resp, err := h.authService.RefreshToken(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "刷新token失败"})
		return
	}

	setAuthCookies(c, resp.Token, time.Until(time.Unix(resp.ExpiresAt, 0)))
	c.JSON(http.StatusOK, TokenResponse{
		ExpiresAt: resp.ExpiresAt,
	})
}

// Logout 清除认证 Cookie。
func (h *AuthHandler) Logout(c *gin.Context) {
	clearAuthCookie(c)
	c.JSON(http.StatusOK, SuccessResponse{Message: "退出成功"})
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
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "获取用户信息失败"})
		return
	}

	c.JSON(http.StatusOK, UserResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
	})
}

// TokenResponse Token响应
type TokenResponse struct {
	ExpiresAt int64 `json:"expires_at"`
}

func setAuthCookies(c *gin.Context, token string, maxAge time.Duration) {
	if maxAge <= 0 {
		maxAge = 7 * 24 * time.Hour
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authCookieName, token, int(maxAge.Seconds()), "/", "", isSecureRequest(c), true)
	c.SetCookie(csrfCookieName, generateCSRFToken(), int(maxAge.Seconds()), "/", "", isSecureRequest(c), false)
}

func clearAuthCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authCookieName, "", -1, "/", "", isSecureRequest(c), true)
	c.SetCookie(csrfCookieName, "", -1, "/", "", isSecureRequest(c), false)
}

func isSecureRequest(c *gin.Context) bool {
	return c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https"
}

func generateCSRFToken() string {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return base64.RawURLEncoding.EncodeToString(buf[:])
}
