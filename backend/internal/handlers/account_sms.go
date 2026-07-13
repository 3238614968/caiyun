package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"caiyun/internal/core/sms"
	"caiyun/internal/services"
	"caiyun/internal/utils"
	apiresponse "caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) SendSmsCode(c *gin.Context) {
	var req SendSmsCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "请输入手机号")
		return
	}
	req.Phone = strings.TrimSpace(req.Phone)

	// 验证手机号格式（11 位，以 1 开头）
	if !phoneRe.MatchString(req.Phone) {
		respondError(c, http.StatusBadRequest, "手机号格式不正确")
		return
	}

	// 检查发送频率限制（优先使用 Redis 分布式限流，回退到内存限流）
	allowed, waitTime := h.checkSMSRateLimit(req.Phone)
	if !allowed {
		log.Printf("[SendSmsCode] 触发频率限制 phone=%s wait=%s", utils.MaskPhone(req.Phone), waitTime)
		respondError(c, http.StatusTooManyRequests, fmt.Sprintf("操作过于频繁，请等待%s后再试", waitTime))
		return
	}

	// 调用 SMS API 发送验证码
	taskID, err := sms.SendCodeContext(c.Request.Context(), req.Phone)
	if err != nil {
		log.Printf("[SendSmsCode] 发送验证码失败 phone=%s: %v", utils.MaskPhone(req.Phone), err)
		h.resetSMSRateLimit(req.Phone)
		// 提供更友好的错误提示
		errMsg := err.Error()
		if strings.Contains(errMsg, "请求失败") || strings.Contains(errMsg, "连接") {
			errMsg = "短信服务暂时不可用，请稍后重试或联系管理员"
		} else if strings.Contains(errMsg, "频率") {
			errMsg = "发送过于频繁，请稍后再试"
		}
		respondError(c, http.StatusInternalServerError, errMsg)
		return
	}

	// 如果taskID为空，说明SMS服务没有返回会话ID
	if taskID == "" {
		log.Printf("[SendSmsCode] SMS服务未返回task_id phone=%s", utils.MaskPhone(req.Phone))
		h.resetSMSRateLimit(req.Phone)
		respondError(c, http.StatusInternalServerError, "短信服务异常：未获取到验证码会话ID，请稍后重试或联系管理员")
		return
	}

	apiresponse.SuccessWithMessage(c, "验证码已发送", map[string]string{
		"phone":   req.Phone,
		"task_id": taskID,
	})
}

// GetSmsStatus 查询验证码发送状态
// @Summary 查询验证码发送状态
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param phone path string true "手机号"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Router /api/accounts/sms/status/{phone} [get]

// GetSmsStatus 查询验证码发送状态
// @Summary 查询验证码发送状态
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param phone path string true "手机号"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Router /api/accounts/sms/status/{phone} [get]
func (h *AccountHandler) GetSmsStatus(c *gin.Context) {
	phone := strings.TrimSpace(c.Param("phone"))

	// 验证手机号格式
	if !phoneRe.MatchString(phone) {
		respondError(c, http.StatusBadRequest, "手机号格式不正确")
		return
	}

	// 调用SMS API查询状态
	statusInfo, err := sms.GetCodeStatusContext(c.Request.Context(), phone)
	if err != nil {
		log.Printf("[GetSmsStatus] 查询状态失败 phone=%s: %v", utils.MaskPhone(phone), err)
		errMsg, retryable := normalizeSMSStatusError(err.Error())
		if retryable {
			h.resetSMSRateLimit(phone)
		}
		apiresponse.Success(c, map[string]interface{}{
			"phone":     phone,
			"status":    "failed",
			"task_id":   "",
			"retryable": retryable,
			"message":   errMsg,
		})
		return
	}

	retryable := false
	statusMessage := ""
	switch statusInfo.Status {
	case "failed":
		retryable = true
		statusMessage = "验证码发送失败，请重新发送"
	case "timeout":
		retryable = true
		statusMessage = "验证码识别超时，请重新发送"
	case "completed":
		statusMessage = "验证码发送成功，请输入验证码"
	}
	if retryable {
		h.resetSMSRateLimit(phone)
	}

	apiresponse.Success(c, map[string]interface{}{
		"phone":     phone,
		"task_id":   statusInfo.TaskID,
		"status":    statusInfo.Status,
		"retryable": retryable,
		"message":   statusMessage,
	})
}

// SmsLogin 短信验证码登录并创建账号
// @Summary 短信验证码登录
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param request body SmsLoginRequest true "短信登录请求"
// @Success 200 {object} models.Account
// @Failure 400 {object} ErrorResponse
// @Router /api/accounts/sms/verify [post]

// SmsLogin 短信验证码登录并创建账号
// @Summary 短信验证码登录
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param request body SmsLoginRequest true "短信登录请求"
// @Success 200 {object} models.Account
// @Failure 400 {object} ErrorResponse
// @Router /api/accounts/sms/verify [post]
func (h *AccountHandler) SmsLogin(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req SmsLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "请输入手机号、验证码，并先发送验证码获取会话")
		return
	}
	req.Phone = strings.TrimSpace(req.Phone)

	// 验证手机号格式
	if !phoneRe.MatchString(req.Phone) {
		respondError(c, http.StatusBadRequest, "手机号格式不正确")
		return
	}

	// 调用SMS API验证验证码，获取authorization
	authorization, err := sms.VerifyCodeContext(c.Request.Context(), req.Phone, req.SmsCode, req.TaskID)
	if err != nil {
		log.Printf("[SmsLogin] 验证失败 phone=%s: %v", utils.MaskPhone(req.Phone), err)
		// 提供更友好的错误提示
		errMsg := err.Error()
		if strings.Contains(errMsg, "请求失败") || strings.Contains(errMsg, "连接") {
			errMsg = "短信服务暂时不可用，请稍后重试"
		} else if strings.Contains(errMsg, "验证码错误") || strings.Contains(errMsg, "不正确") {
			errMsg = "验证码错误，请检查后重试"
		} else if strings.Contains(errMsg, "过期") || strings.Contains(errMsg, "失效") {
			h.resetSMSRateLimit(req.Phone)
			errMsg = "验证码已过期，请重新获取"
		} else if strings.Contains(errMsg, "未找到") || strings.Contains(errMsg, "不存在") {
			h.resetSMSRateLimit(req.Phone)
			errMsg = "验证码会话不存在，请重新发送验证码"
		} else {
			errMsg = "验证失败: " + errMsg
		}
		respondError(c, http.StatusBadRequest, errMsg)
		return
	}

	// 使用获取到的authorization创建账号（复用现有CreateAccount逻辑）
	createReq := &services.CreateAccountRequest{
		Phone:  req.Phone,
		Auth:   authorization,
		Remark: req.Remark,
	}

	account, err := h.accountService.CreateAccountContext(c.Request.Context(), userID, createReq)
	if err != nil {
		if err == services.ErrInvalidPhone {
			respondError(c, http.StatusBadRequest, "手机号格式不正确")
			return
		}
		if err == services.ErrAccountExists {
			respondError(c, http.StatusConflict, "该手机号账号已存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, account)
}
