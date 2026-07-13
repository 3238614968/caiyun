package handlers

import (
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/services"
)

type AccountHandler struct {
	accountService   *services.AccountService
	taskService      *services.TaskService
	operationService *services.OperationService
	smsRateLimiter   *SMSRateLimiter   // 短信发送频率限制器
	redisCache       *cache.RedisCache // 分布式限流用，nil 时回退到内存
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

func (h *AccountHandler) SetOperationService(service *services.OperationService) {
	h.operationService = service
}
