package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	nethttp "net/http"
	"time"
)

// NotificationLevel 通知级别
type NotificationLevel string

const (
	LevelInfo    NotificationLevel = "info"
	LevelWarning NotificationLevel = "warning"
	LevelError   NotificationLevel = "error"
	LevelSuccess NotificationLevel = "success"
)

// Notification 通知消息
type Notification struct {
	Level     NotificationLevel      `json:"level"`
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	AccountID uint                   `json:"account_id"`
	TaskType  string                 `json:"task_type"`
	Timestamp time.Time              `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Notifier 通知器接口
type Notifier interface {
	Send(notification *Notification) error
	SendTaskFailure(accountID uint, taskType, message string) error
	SendTaskSuccess(accountID uint, taskType, message string) error
}

// MultiNotifier 多通知器（可以同时使用多个通知方式）
type MultiNotifier struct {
	notifiers []Notifier
	logger    *log.Logger
}

// NewMultiNotifier 创建多通知器
func NewMultiNotifier(logger *log.Logger) *MultiNotifier {
	return &MultiNotifier{
		notifiers: make([]Notifier, 0),
		logger:    logger,
	}
}

// AddNotifier 添加通知器
func (mn *MultiNotifier) AddNotifier(notifier Notifier) {
	mn.notifiers = append(mn.notifiers, notifier)
}

// Send 发送通知
func (mn *MultiNotifier) Send(notification *Notification) error {
	var lastErr error
	for _, notifier := range mn.notifiers {
		if err := notifier.Send(notification); err != nil {
			mn.logger.Printf("通知发送失败: %v", err)
			lastErr = err
		}
	}
	return lastErr
}

// SendTaskFailure 发送任务失败通知
func (mn *MultiNotifier) SendTaskFailure(accountID uint, taskType, message string) error {
	notification := &Notification{
		Level:     LevelError,
		Title:     fmt.Sprintf("任务执行失败: %s", taskType),
		Message:   message,
		AccountID: accountID,
		TaskType:  taskType,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"event": "task_failure",
		},
	}
	return mn.Send(notification)
}

// SendTaskSuccess 发送任务成功通知
func (mn *MultiNotifier) SendTaskSuccess(accountID uint, taskType, message string) error {
	notification := &Notification{
		Level:     LevelSuccess,
		Title:     fmt.Sprintf("任务执行成功: %s", taskType),
		Message:   message,
		AccountID: accountID,
		TaskType:  taskType,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"event": "task_success",
		},
	}
	return mn.Send(notification)
}

// LogNotifier 日志通知器（默认实现）
type LogNotifier struct {
	logger *log.Logger
}

// NewLogNotifier 创建日志通知器
func NewLogNotifier(logger *log.Logger) *LogNotifier {
	return &LogNotifier{logger: logger}
}

// Send 发送通知到日志
func (ln *LogNotifier) Send(notification *Notification) error {
	levelPrefix := ""
	switch notification.Level {
	case LevelError:
		levelPrefix = "[ERROR]"
	case LevelWarning:
		levelPrefix = "[WARN]"
	case LevelSuccess:
		levelPrefix = "[SUCCESS]"
	default:
		levelPrefix = "[INFO]"
	}

	ln.logger.Printf("%s [账号:%d] [任务:%s] %s: %s",
		levelPrefix,
		notification.AccountID,
		notification.TaskType,
		notification.Title,
		notification.Message,
	)
	return nil
}

// SendTaskFailure 发送任务失败通知
func (ln *LogNotifier) SendTaskFailure(accountID uint, taskType, message string) error {
	return ln.Send(&Notification{
		Level:     LevelError,
		Title:     fmt.Sprintf("任务执行失败: %s", taskType),
		Message:   message,
		AccountID: accountID,
		TaskType:  taskType,
		Timestamp: time.Now(),
	})
}

// SendTaskSuccess 发送任务成功通知
func (ln *LogNotifier) SendTaskSuccess(accountID uint, taskType, message string) error {
	return ln.Send(&Notification{
		Level:     LevelSuccess,
		Title:     fmt.Sprintf("任务执行成功: %s", taskType),
		Message:   message,
		AccountID: accountID,
		TaskType:  taskType,
		Timestamp: time.Now(),
	})
}

// WebhookNotifier Webhook通知器（可以扩展支持钉钉、企业微信等）
type WebhookNotifier struct {
	webhookURL string
	logger     *log.Logger
}

// NewWebhookNotifier 创建Webhook通知器
func NewWebhookNotifier(webhookURL string, logger *log.Logger) *WebhookNotifier {
	return &WebhookNotifier{
		webhookURL: webhookURL,
		logger:     logger,
	}
}

// Send 发送Webhook通知
func (wn *WebhookNotifier) Send(notification *Notification) error {
	if wn.webhookURL == "" {
		return nil
	}

	// 序列化通知
	data, err := json.Marshal(notification)
	if err != nil {
		wn.logger.Printf("序列化通知失败: %v", err)
		return err
	}

	// 发送HTTP POST请求
	resp, err := nethttp.Post(wn.webhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		wn.logger.Printf("Webhook通知发送失败: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		wn.logger.Printf("Webhook通知返回错误状态码: %d", resp.StatusCode)
		return fmt.Errorf("webhook返回状态码: %d", resp.StatusCode)
	}

	return nil
}

// SendTaskFailure 发送任务失败通知
func (wn *WebhookNotifier) SendTaskFailure(accountID uint, taskType, message string) error {
	return wn.Send(&Notification{
		Level:     LevelError,
		Title:     fmt.Sprintf("任务执行失败: %s", taskType),
		Message:   message,
		AccountID: accountID,
		TaskType:  taskType,
		Timestamp: time.Now(),
	})
}

// SendTaskSuccess 发送任务成功通知
func (wn *WebhookNotifier) SendTaskSuccess(accountID uint, taskType, message string) error {
	return wn.Send(&Notification{
		Level:     LevelSuccess,
		Title:     fmt.Sprintf("任务执行成功: %s", taskType),
		Message:   message,
		AccountID: accountID,
		TaskType:  taskType,
		Timestamp: time.Now(),
	})
}
