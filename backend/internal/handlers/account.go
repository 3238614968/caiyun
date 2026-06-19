package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/core/sms"
	"caiyun/internal/models"
	"caiyun/internal/services"
	apiresponse "caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

type AccountHandler struct {
	accountService *services.AccountService
	taskService    *services.TaskService
	smsRateLimiter *SMSRateLimiter   // 短信发送频率限制器
	redisCache     *cache.RedisCache // 分布式限流用，nil 时回退到内存
}

// phoneRe 手机号正则（中国大陆 11 位手机号），包级变量避免每次请求重新编译。
var phoneRe = regexp.MustCompile(`^1[3-9]\d{9}$`)

// SMSRateLimiter 短信发送频率限制器（内存实现）
type SMSRateLimiter struct {
	mu       sync.RWMutex
	records  map[string]*SMSRecord // key: phone
	duration time.Duration         // 时间窗口
	limit    int                   // 最大次数
	stopCh   chan struct{}
	stopOnce sync.Once
}

// SMSRecord 短信发送记录
type SMSRecord struct {
	phone      string
	timestamps []time.Time // 发送时间戳
}

// NewAccountHandler 创建账号处理器
func NewAccountHandler(accountService *services.AccountService, taskService *services.TaskService, redisCache ...*cache.RedisCache) *AccountHandler {
	var rc *cache.RedisCache
	if len(redisCache) > 0 {
		rc = redisCache[0]
	}
	handler := &AccountHandler{
		accountService: accountService,
		taskService:    taskService,
		smsRateLimiter: NewSMSRateLimiter(5*time.Minute, 1), // 5 分钟内最多 1 次
		redisCache:     rc,
	}
	return handler
}

// Close 释放 SMSRateLimiter 的后台清理协程，应在优雅退出时调用。
func (h *AccountHandler) Close() {
	if h == nil {
		return
	}
	if h.smsRateLimiter != nil {
		h.smsRateLimiter.Stop()
	}
}

// NewSMSRateLimiter 创建短信频率限制器
func NewSMSRateLimiter(duration time.Duration, limit int) *SMSRateLimiter {
	limiter := &SMSRateLimiter{
		records:  make(map[string]*SMSRecord),
		duration: duration,
		limit:    limit,
		stopCh:   make(chan struct{}),
	}
	// 启动后台清理任务，每分钟清理一次过期记录
	go limiter.cleanupLoop()
	return limiter
}

// cleanupLoop 定期清理过期记录
func (rl *SMSRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *SMSRateLimiter) Stop() {
	if rl == nil {
		return
	}
	rl.stopOnce.Do(func() {
		close(rl.stopCh)
	})
}

// cleanup 清理过期的发送记录
func (rl *SMSRateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for phone, record := range rl.records {
		// 如果所有时间戳都过期了，删除记录
		if len(record.timestamps) == 0 || now.Sub(record.timestamps[len(record.timestamps)-1]) > rl.duration {
			delete(rl.records, phone)
		}
	}
}

// Allow 检查是否允许发送（返回是否允许及错误信息）
func (rl *SMSRateLimiter) Allow(phone string) (bool, string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	record, exists := rl.records[phone]

	if !exists {
		// 首次发送，创建记录
		rl.records[phone] = &SMSRecord{
			phone:      phone,
			timestamps: []time.Time{now},
		}
		return true, ""
	}

	// 清理过期时间戳
	validTimestamps := make([]time.Time, 0)
	for _, ts := range record.timestamps {
		if now.Sub(ts) < rl.duration {
			validTimestamps = append(validTimestamps, ts)
		}
	}

	// 检查是否在限制内
	if len(validTimestamps) >= rl.limit {
		// 计算还需要等待多久
		waitTime := rl.duration - now.Sub(validTimestamps[0])
		return false, formatWaitTime(waitTime)
	}

	// 允许发送，添加新时间戳
	record.timestamps = append(validTimestamps, now)
	return true, ""
}

// Reset 清理指定手机号的限流记录，用于系统侧发送失败后允许立即重试。
func (rl *SMSRateLimiter) Reset(phone string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.records, phone)
}

// formatWaitTime 格式化等待时间
func formatWaitTime(d time.Duration) string {
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60

	if minutes > 0 {
		return fmt.Sprintf("%d分钟%d秒", minutes, seconds)
	}
	return fmt.Sprintf("%d秒", seconds)
}

// checkSMSRateLimit 检查 SMS 发送频率，优先使用 Redis 分布式限流，回退到内存。
func (h *AccountHandler) checkSMSRateLimit(phone string) (allowed bool, waitTime string) {
	if h.redisCache != nil {
		key := fmt.Sprintf("sms_rate:%s", phone)
		ok, _, ttl, err := h.redisCache.RateLimitCheck(key, 1, 5*time.Minute)
		if err != nil {
			log.Printf("[checkSMSRateLimit] Redis 限流失败，回退内存: %v", err)
			ok2, msg := h.smsRateLimiter.Allow(phone)
			return ok2, msg
		}
		if ok {
			return true, ""
		}
		return false, formatWaitTime(ttl)
	}
	return h.smsRateLimiter.Allow(phone)
}

// resetSMSRateLimit 重置 SMS 限流记录（Redis + 内存双清）。
func (h *AccountHandler) resetSMSRateLimit(phone string) {
	h.smsRateLimiter.Reset(phone)
	if h.redisCache != nil {
		key := fmt.Sprintf("sms_rate:%s", phone)
		if err := h.redisCache.Del(key); err != nil {
			log.Printf("[resetSMSRateLimit] 清除 Redis 限流失败: %v", err)
		}
	}
}

func normalizeSMSStatusError(errMsg string) (message string, retryable bool) {
	errMsg = strings.TrimSpace(errMsg)
	switch {
	case strings.Contains(errMsg, "未找到"), strings.Contains(errMsg, "不存在"):
		return "验证码会话不存在或已过期，请重新发送验证码", true
	case strings.Contains(errMsg, "超限"), strings.Contains(errMsg, "滑动拼图"):
		return "验证码识别失败次数过多，请重新发送验证码", true
	case strings.Contains(errMsg, "过期"), strings.Contains(errMsg, "失效"):
		return "验证码已过期，请重新发送验证码", true
	case strings.Contains(errMsg, "请求失败"), strings.Contains(errMsg, "连接"), strings.Contains(errMsg, "超时"):
		return "短信服务暂时不可用，请重新发送验证码", true
	default:
		if errMsg == "" {
			return "验证码识别失败，请重新发送验证码", true
		}
		return errMsg, true
	}
}

// CreateAccountRequest 创建账号请求 - 复用services中的定义
type CreateAccountRequest = services.CreateAccountRequest

// UpdateAccountRequest 更新账号请求 - 复用services中的定义
type UpdateAccountRequest = services.UpdateAccountRequest

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
func (h *AccountHandler) CreateAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	account, err := h.accountService.CreateAccount(userID, &req)
	if err != nil {
		if err == services.ErrInvalidPhone {
			respondError(c, http.StatusBadRequest, "手机号格式不正确")
			return
		}
		if err == services.ErrAccountExists {
			respondError(c, http.StatusConflict, "账号已存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, account)
}

// ListAccounts 获取账号列表
// @Summary 获取账号列表
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param phone query string false "手机号搜索"
// @Success 200 {object} ListAccountsResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/accounts [get]
func (h *AccountHandler) ListAccounts(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	phone := c.Query("phone")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	accounts, total, err := h.accountService.ListAccounts(userID, page, pageSize, phone)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, ListAccountsResponse{
		Accounts: accounts,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// GetAccount 获取账号详情
// @Summary 获取账号详情
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Success 200 {object} models.Account
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id} [get]
func (h *AccountHandler) GetAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	account, err := h.accountService.GetAccount(userID, uint(accountID))
	if err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, account)
}

// UpdateAccount 更新账号
// @Summary 更新账号
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Param request body UpdateAccountRequest true "更新账号请求"
// @Success 200 {object} models.Account
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id} [put]
func (h *AccountHandler) UpdateAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	account, err := h.accountService.UpdateAccount(userID, uint(accountID), &req)
	if err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		if err == services.ErrInvalidPhone {
			respondError(c, http.StatusBadRequest, "手机号格式不正确")
			return
		}
		if err == services.ErrAccountExists {
			respondError(c, http.StatusConflict, "账号已存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, account)
}

// DeleteAccount 删除账号
// @Summary 删除账号
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Success 200 {object} SuccessResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id} [delete]
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	if err := h.accountService.DeleteAccount(userID, uint(accountID)); err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "删除成功")
}

// SetAccountStatus 设置账号状态
// @Summary 设置账号状态
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Param is_active query bool true "是否激活"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id}/status [put]
func (h *AccountHandler) SetAccountStatus(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	// 获取状态
	isActive := c.Query("is_active") == "true"

	if err := h.accountService.SetAccountStatus(userID, uint(accountID), isActive); err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "状态更新成功")
}

// RefreshToken 刷新账号Token
// @Summary 刷新账号Token
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Success 200 {object} models.Account
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id}/refresh [post]
func (h *AccountHandler) RefreshToken(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	// 获取账号
	account, err := h.accountService.GetAccount(userID, uint(accountID))
	if err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	// 刷新Token
	if err := h.accountService.RefreshToken(account); err != nil {
		// 服务端打日志便于排查 500
		log.Printf("[RefreshToken] account_id=%d phone=%s err=%v", accountID, account.Phone, err)
		respondInternalServer(c)
		return
	}

	// 重新获取更新后的账号信息
	updatedAccount, err := h.accountService.GetAccount(userID, uint(accountID))
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, updatedAccount)
}

// TriggerTask 手动触发任务执行
// @Summary 手动触发任务执行
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Success 200 {object} SuccessResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id}/trigger [post]
func (h *AccountHandler) TriggerTask(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	// 验证账号存在性和权限
	account, err := h.accountService.GetAccount(userID, uint(accountID))
	if err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	// 检查今日是否已手动执行过
	if h.taskService.HasExecutedToday(account.ID) {
		respondError(c, http.StatusTooManyRequests, "该账号今日已执行过任务，每天限手动执行一次")
		return
	}

	// 自动刷新Token（如果需要）
	if err := h.accountService.RefreshTokenIfNeeded(account); err != nil {
		log.Printf("[TriggerTask] 刷新Token失败 account_id=%d: %v", accountID, err)
		// Token刷新失败不阻止任务执行，继续尝试
	}

	// 直接在goroutine中执行任务（不依赖worker进程），添加超时控制防止永久阻塞。
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[TriggerTask] 账号 %d 执行 panic: %v\n%s", account.ID, r, debug.Stack())
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		log.Printf("[TriggerTask] 开始执行账号 %d 的任务", account.ID)
		if _, err := h.taskService.ExecuteTaskForAccountContext(ctx, account); err != nil {
			log.Printf("[TriggerTask] 账号 %d 任务执行失败: %v", account.ID, err)
		} else {
			log.Printf("[TriggerTask] 账号 %d 任务执行完成", account.ID)
		}
	}()

	apiresponse.Message(c, "任务已开始执行")
}

// SendSmsCodeRequest 发送短信验证码请求
type SendSmsCodeRequest struct {
	Phone string `json:"phone" binding:"required"`
}

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
		log.Printf("[SendSmsCode] 触发频率限制 phone=%s wait=%s", req.Phone, waitTime)
		respondError(c, http.StatusTooManyRequests, fmt.Sprintf("操作过于频繁，请等待%s后再试", waitTime))
		return
	}

	// 调用 SMS API 发送验证码
	taskID, err := sms.SendCode(req.Phone)
	if err != nil {
		log.Printf("[SendSmsCode] 发送验证码失败 phone=%s: %v", req.Phone, err)
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
		log.Printf("[SendSmsCode] SMS服务未返回task_id phone=%s", req.Phone)
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
func (h *AccountHandler) GetSmsStatus(c *gin.Context) {
	phone := strings.TrimSpace(c.Param("phone"))

	// 验证手机号格式
	if !phoneRe.MatchString(phone) {
		respondError(c, http.StatusBadRequest, "手机号格式不正确")
		return
	}

	// 调用SMS API查询状态
	statusInfo, err := sms.GetCodeStatus(phone)
	if err != nil {
		log.Printf("[GetSmsStatus] 查询状态失败 phone=%s: %v", phone, err)
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
	authorization, err := sms.VerifyCode(req.Phone, req.SmsCode, req.TaskID)
	if err != nil {
		log.Printf("[SmsLogin] 验证失败 phone=%s: %v", req.Phone, err)
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

	account, err := h.accountService.CreateAccount(userID, createReq)
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
