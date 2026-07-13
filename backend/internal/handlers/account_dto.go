package handlers

import (
	"caiyun/internal/models"
	"caiyun/internal/services"
)

type CreateAccountRequest = services.CreateAccountRequest

// UpdateAccountRequest 更新账号请求 - 复用services中的定义

// UpdateAccountRequest 更新账号请求 - 复用services中的定义
type UpdateAccountRequest = services.UpdateAccountRequest

// ListAccountsResponse 账号列表响应

// ListAccountsResponse 账号列表响应
type ListAccountsResponse struct {
	Accounts []*models.Account `json:"accounts"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// CreateAccount 创建账号
// @Summary 创建账号
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param request body CreateAccountRequest true "创建账号请求"
// @Success 200 {object} models.Account
// @Failure 400 {object} ErrorResponse
// @Router /api/accounts [post]

// SendSmsCodeRequest 发送短信验证码请求
type SendSmsCodeRequest struct {
	Phone string `json:"phone" binding:"required"`
}

// SmsLoginRequest 短信验证码登录请求

// SmsLoginRequest 短信验证码登录请求
type SmsLoginRequest struct {
	Phone   string `json:"phone" binding:"required"`
	SmsCode string `json:"sms_code" binding:"required"`
	TaskID  string `json:"task_id" binding:"required"`
	Remark  string `json:"remark"`
}

// SendSmsCode 发送短信验证码
// @Summary 发送短信验证码
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param request body SendSmsCodeRequest true "发送短信验证码请求"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 429 {object} ErrorResponse  // 频率限制
// @Router /api/accounts/sms/send [post]
