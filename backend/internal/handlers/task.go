package handlers

import (
	"caiyun/internal/models"
	"caiyun/internal/monitor"
	"caiyun/internal/queue"
	"caiyun/internal/services"
	"caiyun/pkg/response"
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type TaskHandler struct {
	taskService    *services.TaskService
	cloudService   *services.CloudService
	accountService *services.AccountService
	redisCache     interface {
		LLen(key string) int64
		ZCard(key string) int64
	}
}

func NewTaskHandler(taskService *services.TaskService, cloudService *services.CloudService, accountService *services.AccountService) *TaskHandler {
	return &TaskHandler{
		taskService:    taskService,
		cloudService:   cloudService,
		accountService: accountService,
	}
}

// SetRedisCache 设置Redis缓存（用于获取队列状态）
func (h *TaskHandler) SetRedisCache(cache interface {
	LLen(key string) int64
	ZCard(key string) int64
}) {
	h.redisCache = cache
}

// TaskLogsResponse 任务日志响应
type TaskLogsResponse struct {
	TaskLogs []*models.TaskLog `json:"task_logs"`
	Total    int64             `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
}

// GetTaskLogs 获取任务日志
// @Summary 获取任务日志
// @Tags 任务管理
// @Accept json
// @Produce json
// @Param account_id query int false "账号ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} TaskLogsResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/tasks/logs [get]
func (h *TaskHandler) GetTaskLogs(c *gin.Context) {
	// 获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 获取账号ID（可选）
	var accountID *uint
	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		id, err := strconv.ParseUint(accountIDStr, 10, 32)
		if err != nil {
			respondError(c, http.StatusBadRequest, "无效的账号ID")
			return
		}
		accountIDUint := uint(id)
		accountID = &accountIDUint
	}

	taskLogs, total, err := h.taskService.GetTaskLogs(userID.(uint), accountID, page, pageSize)
	if err != nil {
		respondInternalServer(c)
		return
	}

	response.Success(c, TaskLogsResponse{
		TaskLogs: taskLogs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// DashboardDataResponse 仪表盘数据响应
type DashboardDataResponse struct {
	Data *services.DashboardData `json:"data"`
}

// GetDashboard 获取仪表盘数据
// @Summary 获取仪表盘数据
// @Tags 数据统计
// @Accept json
// @Produce json
// @Success 200 {object} DashboardDataResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/stats/dashboard [get]
func (h *TaskHandler) GetDashboard(c *gin.Context) {
	// 获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	dashboard, err := h.cloudService.GetDashboard(userID.(uint))
	if err != nil {
		respondInternalServer(c)
		return
	}

	response.Success(c, dashboard)
}

// CloudStatsResponse 云朵统计响应
type CloudStatsResponse struct {
	CloudStats []*models.CloudStats `json:"cloud_stats"`
	Total      int64                `json:"total"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
}

// GetCloudStats 获取云朵统计
// @Summary 获取云朵统计
// @Tags 数据统计
// @Accept json
// @Produce json
// @Param account_id query int false "账号ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Success 200 {object} CloudStatsResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/stats/cloud [get]
func (h *TaskHandler) GetCloudStats(c *gin.Context) {
	// 获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 获取账号ID（可选）
	var cloudStats []*models.CloudStats
	var total int64
	var err error

	if accountIDStr := c.Query("account_id"); accountIDStr != "" {
		// 获取指定账号的统计
		accountID, err := strconv.ParseUint(accountIDStr, 10, 32)
		if err != nil {
			respondError(c, http.StatusBadRequest, "无效的账号ID")
			return
		}
		cloudStats, total, err = h.cloudService.GetCloudStatsByAccount(userID.(uint), uint(accountID), page, pageSize)
	} else {
		// 获取用户的所有统计
		cloudStats, total, err = h.cloudService.GetCloudStatsByUserID(userID.(uint), page, pageSize)
	}

	if err != nil {
		respondInternalServer(c)
		return
	}

	response.Success(c, CloudStatsResponse{
		CloudStats: cloudStats,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
	})
}

// GetTrendData 获取趋势数据
// @Summary 获取趋势数据
// @Tags 数据统计
// @Accept json
// @Produce json
// @Param days query int false "天数" default(7)
// @Success 200 {object} TrendDataResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/stats/trend [get]
func (h *TaskHandler) GetTrendData(c *gin.Context) {
	// 获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	// 获取天数参数
	days, _ := strconv.Atoi(c.DefaultQuery("days", "7"))
	if days < 1 || days > 365 {
		days = 7
	}

	var trendData []services.TrendPoint
	var err error
	if role, _ := c.Get("role"); role == "admin" {
		trendData, err = h.cloudService.GetGlobalTrendData(days)
	} else {
		trendData, err = h.cloudService.GetTrendData(userID.(uint), days)
	}
	if err != nil {
		respondInternalServer(c)
		return
	}

	response.Success(c, gin.H{"trend_data": trendData})
}

// TrendDataResponse 趋势数据响应
type TrendDataResponse struct {
	TrendData []services.TrendPoint `json:"trend_data"`
}

// TriggerAllTasks 触发所有账号的任务执行
// @Summary 触发所有账号的任务执行
// @Description 异步触发当前用户所有激活账号的任务执行
// @Tags 任务管理
// @Accept json
// @Produce json
// @Success 200 {object} SuccessResponse "任务提交成功"
// @Failure 401 {object} ErrorResponse "未授权"
// @Security BearerAuth
// @Router /api/tasks/trigger-all [post]
func (h *TaskHandler) TriggerAllTasks(c *gin.Context) {
	// 获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	// 获取用户的所有激活账号
	accounts, err := h.accountService.GetActiveAccounts(userID.(uint))
	if err != nil {
		respondInternalServer(c)
		return
	}

	// 过滤今日未执行的账号
	var toExecute []*models.Account
	dailyTaskTypes := h.taskService.DailyTaskTypes()
	for _, account := range accounts {
		if !h.taskService.HasExecutedTodayForTaskTypes(account.ID, dailyTaskTypes) {
			toExecute = append(toExecute, account)
		}
	}

	if len(toExecute) == 0 {
		response.Message(c, "所有账号今日已执行过任务")
		return
	}

	// 使用信号量控制并发数（最多3个账号同时执行，防止内存暴涨）
	go func() {
		const maxConcurrency = 3
		sem := make(chan struct{}, maxConcurrency)
		var wg sync.WaitGroup

		for _, account := range toExecute {
			sem <- struct{}{} // 获取信号量
			wg.Add(1)

			go func(acc *models.Account) {
				defer wg.Done()
				defer func() { <-sem }() // 释放信号量
				tm := monitor.GetGlobalTaskMonitor()
				const monitorTaskType = "trigger_all"
				if tm != nil {
					_ = tm.StartTask(acc.ID, monitorTaskType, 0)
					_ = tm.UpdateTaskProgress(acc.ID, monitorTaskType, 0.1, "开始执行账号任务")
				}

				// panic recovery，防止单个账号崩溃导致整个进程退出
				defer func() {
					if r := recover(); r != nil {
						// 打印完整堆栈信息，便于定位 nil pointer 等崩溃位置
						fmt.Printf("[TriggerAll] 账号 %d 执行 panic: %v\n%s\n", acc.ID, r, debug.Stack())
						if tm != nil {
							_ = tm.FailTask(acc.ID, monitorTaskType, fmt.Errorf("panic: %v", r))
						}
					}
				}()

				fmt.Printf("[TriggerAll] 开始执行账号 %d 的任务\n", acc.ID)
				// 自动刷新Token
				_ = h.accountService.RefreshTokenIfNeeded(acc)
				if tm != nil {
					_ = tm.UpdateTaskProgress(acc.ID, monitorTaskType, 0.5, "账号鉴权刷新完成，开始执行任务")
				}
				// 执行任务，限制单账号执行时间，避免上游接口卡住导致后台 goroutine 永久占用。
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				if _, err := h.taskService.ExecuteTaskForAccountContext(ctx, acc); err != nil {
					fmt.Printf("[TriggerAll] 账号 %d 执行失败: %v\n", acc.ID, err)
					if tm != nil {
						_ = tm.FailTask(acc.ID, monitorTaskType, err)
					}
				} else {
					fmt.Printf("[TriggerAll] 账号 %d 任务执行完成\n", acc.ID)
					if tm != nil {
						_ = tm.CompleteTask(acc.ID, monitorTaskType, true, "账号任务执行完成")
					}
				}
			}(account)
		}

		wg.Wait()
		fmt.Printf("[TriggerAll] 全部 %d 个账号执行完成\n", len(toExecute))
	}()

	response.Message(c, fmt.Sprintf("已开始执行 %d 个账号的任务（%d 个已跳过），并发限制 3", len(toExecute), len(accounts)-len(toExecute)))
}

// CalculateStats 手动计算统计数据
// @Summary 手动计算统计数据
// @Tags 数据统计
// @Accept json
// @Produce json
// @Success 200 {object} SuccessResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/stats/calculate [post]
func (h *TaskHandler) CalculateStats(c *gin.Context) {
	// 获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	if role, _ := c.Get("role"); role == "admin" {
		// 管理员首页展示全局数据，手动计算时同步刷新全站账号快照。
		if err := h.cloudService.CalculateDailyStats(); err != nil {
			respondInternalServer(c)
			return
		}
	} else {
		// 普通用户仅计算自己的每日统计，避免触发全站账号重算。
		if err := h.cloudService.CalculateDailyStatsByUserID(userID.(uint)); err != nil {
			respondInternalServer(c)
			return
		}

		// 更新差异值
		if err := h.cloudService.UpdateCloudDiffs(userID.(uint)); err != nil {
			respondInternalServer(c)
			return
		}
	}

	response.Message(c, "统计数据计算完成")
}

// GetTotalCloudCount 获取总云朵数
// @Summary 获取总云朵数
// @Tags 数据统计
// @Accept json
// @Produce json
// @Success 200 {object} TotalCloudCountResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/stats/total-cloud [get]
func (h *TaskHandler) GetTotalCloudCount(c *gin.Context) {
	// 获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	total, err := h.cloudService.GetTotalCloudCount(userID.(uint))
	if err != nil {
		respondInternalServer(c)
		return
	}

	response.Success(c, gin.H{"total_cloud": total})
}

// TotalCloudCountResponse 总云朵数响应
type TotalCloudCountResponse struct {
	TotalCloud int `json:"total_cloud"`
}

// QueueStatusResponse 队列状态响应
type QueueStatusResponse struct {
	QueueLength     int64                   `json:"queue_length"`
	ProcessingCount int64                   `json:"processing_count"`
	DelayedCount    int64                   `json:"delayed_count"`
	DeadLetterCount int64                   `json:"dead_letter_count"`
	ActiveWorkers   int32                   `json:"active_workers"`
	PendingTasks    int                     `json:"pending_tasks"`
	CompletedTasks  int32                   `json:"completed_tasks"`
	SuccessfulTasks int32                   `json:"successful_tasks"`
	FailedTasks     int32                   `json:"failed_tasks"`
	Backend         string                  `json:"backend"`
	BackendMeta     queue.TaskQueueMetadata `json:"backend_meta"`
	IsHealthy       bool                    `json:"is_healthy"`
	Errors          []string                `json:"errors,omitempty"`
}

// GetQueueStatus 获取队列状态
// @Summary 获取队列状态
// @Tags 任务管理
// @Accept json
// @Produce json
// @Success 200 {object} QueueStatusResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/tasks/queue-status [get]
func (h *TaskHandler) GetQueueStatus(c *gin.Context) {
	var queueLength int64
	var processingLength int64
	var delayedLength int64
	var deadLetterLength int64
	if h.redisCache != nil {
		queueLength = h.redisCache.LLen(queue.TaskQueueKey)
		processingLength = h.redisCache.LLen(queue.TaskProcessingKey)
		delayedLength = h.redisCache.ZCard(queue.TaskDelayedKey)
		deadLetterLength = h.redisCache.LLen(queue.TaskDeadLetterKey)
	}

	response.Success(c, QueueStatusResponse{
		QueueLength:     queueLength,
		ProcessingCount: processingLength,
		DelayedCount:    delayedLength,
		DeadLetterCount: deadLetterLength,
		ActiveWorkers:   0,
		PendingTasks:    int(queueLength + processingLength + delayedLength),
		CompletedTasks:  0,
		SuccessfulTasks: 0,
		FailedTasks:     0,
		Backend:         queue.TaskQueueBackendList,
		BackendMeta: queue.TaskQueueMetadata{
			Backend:       queue.TaskQueueBackendList,
			PendingKey:    queue.TaskQueueKey,
			ProcessingKey: queue.TaskProcessingKey,
			DelayedKey:    queue.TaskDelayedKey,
			DeadLetterKey: queue.TaskDeadLetterKey,
		},
		IsHealthy: true,
	})
}

// TaskStatusResponse 任务状态响应
type TaskStatusResponse struct {
	Tasks []TaskStatusItem `json:"tasks"`
}

// TaskStatusItem 任务状态项
type TaskStatusItem struct {
	AccountID uint    `json:"account_id"`
	TaskType  string  `json:"task_type"`
	Status    string  `json:"status"`
	Progress  float64 `json:"progress"`
	Message   string  `json:"message"`
	StartTime string  `json:"start_time,omitempty"`
	EndTime   string  `json:"end_time,omitempty"`
}

// GetTaskStatus 获取任务状态
// @Summary 获取任务状态
// @Tags 任务管理
// @Accept json
// @Produce json
// @Param account_id query int false "账号ID"
// @Success 200 {object} TaskStatusResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/tasks/status [get]
func (h *TaskHandler) GetTaskStatus(c *gin.Context) {
	// 从最近的任务日志获取状态
	userID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	// 获取最近的任务日志作为任务状态
	logs, _, err := h.taskService.GetTaskLogs(userID.(uint), nil, 1, 20)
	if err != nil {
		response.Success(c, TaskStatusResponse{Tasks: []TaskStatusItem{}})
		return
	}

	var items []TaskStatusItem
	for _, log := range logs {
		progress := 0.0
		if log.Status == "success" {
			progress = 1.0
		} else if log.Status == "failed" {
			progress = 1.0
		}
		items = append(items, TaskStatusItem{
			AccountID: log.AccountID,
			TaskType:  log.TaskType,
			Status:    log.Status,
			Progress:  progress,
			Message:   log.Message,
			StartTime: log.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	response.Success(c, TaskStatusResponse{Tasks: items})
}
