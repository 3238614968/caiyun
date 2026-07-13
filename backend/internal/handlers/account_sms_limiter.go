package handlers

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
)

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
