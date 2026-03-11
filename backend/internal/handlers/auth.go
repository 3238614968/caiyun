package handlers

import (
	"net/http"

	"caiyun/internal/services"
	"caiyun/pkg/jwt"
	"github.com/gin-gonic/gin"
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

type AuthResponse struct {
	Token     string       `json:"token"`
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
// @Success 201 {object} services.AuthResponse
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
		switch err {
		case services.ErrUserExists:
			c.JSON(http.StatusConflict, ErrorResponse{Message: "用户名已存在"})
		case services.ErrEmailExists:
			c.JSON(http.StatusConflict, ErrorResponse{Message: "邮箱已被注册"})
		default:
			c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		}
		return
	}

	c.JSON(http.StatusCreated, AuthResponse{
		Token:     resp.Token,
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
// @Success 200 {object} services.AuthResponse
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

	c.JSON(http.StatusOK, AuthResponse{
		Token:     resp.Token,
		ExpiresAt: resp.ExpiresAt,
		User: UserResponse{
			ID:       resp.User.ID,
			Username: resp.User.Username,
			Email:    resp.User.Email,
			Role:     resp.User.Role,
		},
	})
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

	c.JSON(http.StatusOK, TokenResponse{
		Token:     resp.Token,
		ExpiresAt: resp.ExpiresAt,
	})
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
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}
