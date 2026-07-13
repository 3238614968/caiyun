package monitor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// TaskStatus 任务状态
type TaskStatus struct {
	AccountID  uint
	TaskType   string
	Status     string // pending, running, success, failed, retrying
	StartTime  time.Time
	EndTime    time.Time
	RetryCount int
	MaxRetries int
	LastError  string
	Progress   float64
	Message    string
}

// TaskMonitor 任务监控器
type TaskMonitor struct {
	tasks       map[string]*TaskStatus // key: accountID_taskType
	taskHistory []*TaskStatus
	mu          sync.RWMutex
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *log.Logger
	maxHistory  int
	activeTasks int32
	completed   int32
	successful  int32
	failed      int32
	subscribers []chan *TaskStatus
	subMu       sync.RWMutex
}

// Config 监控器配置
type Config struct {
	MaxHistory int
	Logger     *log.Logger
}

// NewTaskMonitor 创建任务监控器
func NewTaskMonitor(config ...Config) *TaskMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	var cfg Config
	if len(config) > 0 {
		cfg = config[0]
	}

	if cfg.MaxHistory == 0 {
		cfg.MaxHistory = 1000
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}

	return &TaskMonitor{
		tasks:       make(map[string]*TaskStatus),
		taskHistory: make([]*TaskStatus, 0, cfg.MaxHistory),
		ctx:         ctx,
		cancel:      cancel,
		logger:      cfg.Logger,
		maxHistory:  cfg.MaxHistory,
		subscribers: make([]chan *TaskStatus, 0),
	}
}

// StartTask 开始任务
func (tm *TaskMonitor) StartTask(accountID uint, taskType string, maxRetries int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := tm.getTaskKey(accountID, taskType)

	// 检查是否已有运行中的任务
	if existing, exists := tm.tasks[key]; exists && existing.Status == "running" {
		return fmt.Errorf("任务正在运行中: %s", key)
	}

	status := &TaskStatus{
		AccountID:  accountID,
		TaskType:   taskType,
		Status:     "running",
		StartTime:  time.Now(),
		MaxRetries: maxRetries,
		Progress:   0,
	}

	tm.tasks[key] = status
	atomic.AddInt32(&tm.activeTasks, 1)

	tm.logger.Printf("任务开始: %s", key)
	tm.notifySubscribers(status)

	return nil
}

// UpdateTaskProgress 更新任务进度
func (tm *TaskMonitor) UpdateTaskProgress(accountID uint, taskType string, progress float64, message string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := tm.getTaskKey(accountID, taskType)
	status, exists := tm.tasks[key]
	if !exists {
		return fmt.Errorf("任务不存在: %s", key)
	}

	if status.Status != "running" {
		return fmt.Errorf("任务状态不允许更新进度: %s", status.Status)
	}

	status.Progress = progress
	status.Message = message

	tm.logger.Printf("任务进度更新: %s - %.1f%% - %s", key, progress*100, message)
	tm.notifySubscribers(status)

	return nil
}

// CompleteTask 完成任务
func (tm *TaskMonitor) CompleteTask(accountID uint, taskType string, success bool, message string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := tm.getTaskKey(accountID, taskType)
	status, exists := tm.tasks[key]
	if !exists {
		return fmt.Errorf("任务不存在: %s", key)
	}

	status.EndTime = time.Now()
	status.Message = message

	if success {
		status.Status = "success"
		atomic.AddInt32(&tm.successful, 1)
		tm.logger.Printf("任务成功完成: %s - %s", key, message)
	} else {
		status.Status = "failed"
		atomic.AddInt32(&tm.failed, 1)
		tm.logger.Printf("任务失败: %s - %s", key, message)
	}

	atomic.AddInt32(&tm.completed, 1)
	atomic.AddInt32(&tm.activeTasks, -1)

	// 保存到历史记录
	tm.addToHistory(status)

	tm.notifySubscribers(status)

	return nil
}

// FailTask 任务失败
func (tm *TaskMonitor) FailTask(accountID uint, taskType string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return tm.CompleteTask(accountID, taskType, false, message)
}

// RetryTask 重试任务
func (tm *TaskMonitor) RetryTask(accountID uint, taskType string, err error) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	key := tm.getTaskKey(accountID, taskType)
	status, exists := tm.tasks[key]
	if !exists {
		return fmt.Errorf("任务不存在: %s", key)
	}

	if status.RetryCount >= status.MaxRetries {
		status.Status = "failed"
		status.LastError = err.Error()
		status.EndTime = time.Now()

		atomic.AddInt32(&tm.failed, 1)
		atomic.AddInt32(&tm.completed, 1)
		atomic.AddInt32(&tm.activeTasks, -1)

		tm.addToHistory(status)
		tm.logger.Printf("任务重试次数已达上限: %s - %s", key, err.Error())
		tm.notifySubscribers(status)

		return fmt.Errorf("重试次数已达上限: %d", status.MaxRetries)
	}

	status.Status = "retrying"
	status.RetryCount++
	status.LastError = err.Error()
	status.Message = fmt.Sprintf("第 %d 次重试", status.RetryCount)

	tm.logger.Printf("任务重试: %s - 第 %d 次", key, status.RetryCount)
	tm.notifySubscribers(status)

	return nil
}

// GetTaskStatus 获取任务状态
func (tm *TaskMonitor) GetTaskStatus(accountID uint, taskType string) (*TaskStatus, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	key := tm.getTaskKey(accountID, taskType)
	status, exists := tm.tasks[key]
	if !exists {
		return nil, fmt.Errorf("任务不存在: %s", key)
	}

	return status, nil
}

// GetActiveTasks 获取活跃任务列表
func (tm *TaskMonitor) GetActiveTasks() []*TaskStatus {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	var active []*TaskStatus
	for _, status := range tm.tasks {
		if status.Status == "running" || status.Status == "retrying" {
			active = append(active, status)
		}
	}

	return active
}

// GetTaskHistory 获取任务历史
func (tm *TaskMonitor) GetTaskHistory(limit int) []*TaskStatus {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if limit <= 0 || limit > len(tm.taskHistory) {
		limit = len(tm.taskHistory)
	}

	start := len(tm.taskHistory) - limit
	if start < 0 {
		start = 0
	}

	return tm.taskHistory[start:]
}

// GetStats 获取统计信息
func (tm *TaskMonitor) GetStats() map[string]interface{} {
	active := atomic.LoadInt32(&tm.activeTasks)
	completed := atomic.LoadInt32(&tm.completed)
	successful := atomic.LoadInt32(&tm.successful)
	failed := atomic.LoadInt32(&tm.failed)

	total := active + completed
	successRate := 0.0
	if total > 0 {
		successRate = float64(successful) / float64(total) * 100
	}

	return map[string]interface{}{
		"active_tasks":    active,
		"completed_tasks": completed,
		"successful":      successful,
		"failed":          failed,
		"success_rate":    fmt.Sprintf("%.1f%%", successRate),
		"total_tasks":     total,
	}
}

// Subscribe 订阅任务状态更新
func (tm *TaskMonitor) Subscribe() <-chan *TaskStatus {
	tm.subMu.Lock()
	defer tm.subMu.Unlock()

	ch := make(chan *TaskStatus, 100)
	tm.subscribers = append(tm.subscribers, ch)

	return ch
}

// Unsubscribe 取消订阅
func (tm *TaskMonitor) Unsubscribe(ch <-chan *TaskStatus) {
	tm.subMu.Lock()
	defer tm.subMu.Unlock()

	for i, subscriber := range tm.subscribers {
		if subscriber == ch {
			tm.subscribers = append(tm.subscribers[:i], tm.subscribers[i+1:]...)
			close(subscriber)
			break
		}
	}
}

// Stop 停止监控器
func (tm *TaskMonitor) Stop() {
	tm.logger.Println("正在停止任务监控器...")
	tm.cancel()

	// 关闭所有订阅者
	tm.subMu.Lock()
	for _, subscriber := range tm.subscribers {
		close(subscriber)
	}
	tm.subscribers = make([]chan *TaskStatus, 0)
	tm.subMu.Unlock()

	tm.logger.Println("任务监控器已停止")
}

// CleanupStaleTasks 清理过期任务
func (tm *TaskMonitor) CleanupStaleTasks(timeout time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	cleaned := 0

	for key, status := range tm.tasks {
		if status.Status == "running" && now.Sub(status.StartTime) > timeout {
			status.Status = "failed"
			status.EndTime = now
			status.Message = "任务超时"

			atomic.AddInt32(&tm.failed, 1)
			atomic.AddInt32(&tm.completed, 1)
			atomic.AddInt32(&tm.activeTasks, -1)

			tm.addToHistory(status)
			tm.notifySubscribers(status)

			delete(tm.tasks, key)
			cleaned++
		}
	}

	if cleaned > 0 {
		tm.logger.Printf("清理了 %d 个过期任务", cleaned)
	}
}

// StartCleanupJob 启动清理任务
func (tm *TaskMonitor) StartCleanupJob(interval, timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tm.CleanupStaleTasks(timeout)
			case <-tm.ctx.Done():
				return
			}
		}
	}()
}

// 辅助方法
func (tm *TaskMonitor) getTaskKey(accountID uint, taskType string) string {
	return fmt.Sprintf("%d_%s", accountID, taskType)
}

func (tm *TaskMonitor) addToHistory(status *TaskStatus) {
	tm.taskHistory = append(tm.taskHistory, status)

	// 限制历史记录数量
	if len(tm.taskHistory) > tm.maxHistory {
		tm.taskHistory = tm.taskHistory[len(tm.taskHistory)-tm.maxHistory:]
	}
}

func (tm *TaskMonitor) notifySubscribers(status *TaskStatus) {
	tm.subMu.RLock()
	defer tm.subMu.RUnlock()

	// 创建副本避免竞态条件
	statusCopy := *status

	for _, subscriber := range tm.subscribers {
		select {
		case subscriber <- &statusCopy:
		default:
			tm.logger.Printf("订阅者通道已满，丢弃状态更新: %s", statusCopy.TaskType)
		}
	}
}

// RetryManager 重试管理器
type RetryManager struct {
	monitor     *TaskMonitor
	maxRetries  int
	retryDelay  time.Duration
	backoffFunc func(attempt int) time.Duration
}

// NewRetryManager 创建重试管理器
func NewRetryManager(monitor *TaskMonitor, maxRetries int, retryDelay time.Duration) *RetryManager {
	return &RetryManager{
		monitor:    monitor,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		backoffFunc: func(attempt int) time.Duration {
			return retryDelay * time.Duration(attempt)
		},
	}
}

// ExecuteWithRetry 带重试的任务执行
func (rm *RetryManager) ExecuteWithRetry(
	accountID uint,
	taskType string,
	taskFunc func() error,
	onProgress func(float64, string),
) error {
	// 开始任务
	if err := rm.monitor.StartTask(accountID, taskType, rm.maxRetries); err != nil {
		return err
	}

	var lastErr error

	for attempt := 0; attempt <= rm.maxRetries; attempt++ {
		if attempt > 0 {
			// 重试逻辑
			if err := rm.monitor.RetryTask(accountID, taskType, lastErr); err != nil {
				return err
			}

			// 指数退避
			delay := rm.backoffFunc(attempt)
			time.Sleep(delay)
		}

		// 更新进度
		if onProgress != nil {
			onProgress(0.1, "开始执行任务")
		}

		// 执行任务
		if err := taskFunc(); err != nil {
			lastErr = err

			if onProgress != nil {
				onProgress(0.5, fmt.Sprintf("第 %d 次执行失败: %v", attempt+1, err))
			}

			if attempt == rm.maxRetries {
				// 最后一次尝试失败
				rm.monitor.FailTask(accountID, taskType, err)
				return err
			}

			continue
		}

		// 任务成功
		if onProgress != nil {
			onProgress(1.0, "任务执行成功")
		}

		rm.monitor.CompleteTask(accountID, taskType, true, "任务执行成功")
		return nil
	}

	return lastErr
}
