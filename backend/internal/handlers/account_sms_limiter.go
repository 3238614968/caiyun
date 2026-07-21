package handlers

import (
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type smsSession struct {
	UserID    uint      `json:"user_id"`
	TaskID    string    `json:"task_id"`
	Phone     string    `json:"phone"`
	CreatedAt time.Time `json:"created_at"`
}

const smsSessionTTL = 10 * time.Minute

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
func (h *AccountHandler) checkSMSRateLimit(userID uint, phone string) (allowed bool, waitTime string) {
	key := fmt.Sprintf("sms_rate:%d:%s", userID, phone)
	if h.redisCache != nil {
		ok, _, ttl, err := h.redisCache.RateLimitCheck(key, 1, 5*time.Minute)
		if err != nil {
			log.Printf("[checkSMSRateLimit] Redis 限流失败: %v", err)
			if smsRateLimitFailClosed() {
				return false, "稍后重试"
			}
			ok2, msg := h.smsRateLimiter.Allow(fmt.Sprintf("%d:%s", userID, phone))
			return ok2, msg
		}
		if ok {
			return true, ""
		}
		return false, formatWaitTime(ttl)
	}
	if smsRateLimitFailClosed() {
		log.Printf("[checkSMSRateLimit] 生产环境未配置 Redis 限流")
		return false, "稍后重试"
	}
	return h.smsRateLimiter.Allow(fmt.Sprintf("%d:%s", userID, phone))
}

func smsRateLimitFailClosed() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("APP_ENV")), "production")
}

// resetSMSRateLimit 重置 SMS 限流记录（Redis + 内存双清）。
func (h *AccountHandler) resetSMSRateLimit(userID uint, phone string) {
	h.smsRateLimiter.Reset(fmt.Sprintf("%d:%s", userID, phone))
	if h.redisCache != nil {
		key := fmt.Sprintf("sms_rate:%d:%s", userID, phone)
		if err := h.redisCache.Del(key); err != nil {
			log.Printf("[resetSMSRateLimit] 清除 Redis 限流失败: %v", err)
		}
	}
}

func smsSessionKey(userID uint, taskID string) string {
	return fmt.Sprintf("sms_session:%d:%s", userID, taskID)
}

func (h *AccountHandler) saveSMSSession(userID uint, taskID, phone string) error {
	if h.redisCache == nil {
		return errors.New("短信会话存储未配置")
	}
	return h.redisCache.Set(smsSessionKey(userID, taskID), smsSession{
		UserID: userID, TaskID: taskID, Phone: phone, CreatedAt: time.Now(),
	}, smsSessionTTL)
}

func (h *AccountHandler) loadSMSSession(userID uint, taskID string) (*smsSession, error) {
	if h.redisCache == nil {
		return nil, errors.New("短信会话存储未配置")
	}
	var session smsSession
	if err := h.redisCache.Get(smsSessionKey(userID, taskID), &session); err != nil {
		return nil, err
	}
	if session.UserID != userID || session.TaskID != taskID || !phoneRe.MatchString(session.Phone) {
		return nil, errors.New("短信会话归属校验失败")
	}
	return &session, nil
}

func (h *AccountHandler) deleteSMSSession(userID uint, taskID string) error {
	if h.redisCache == nil {
		return errors.New("短信会话存储未配置")
	}
	return h.redisCache.Del(smsSessionKey(userID, taskID))
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
		return "验证码识别失败，请重新发送验证码", true
	}
}

// CreateAccountRequest 创建账号请求 - 复用services中的定义
