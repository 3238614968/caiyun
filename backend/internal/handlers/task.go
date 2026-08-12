package handlers

import (
	"caiyun/internal/dto"
	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/services"
	appErrors "caiyun/pkg/errors"
	"caiyun/pkg/response"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type TaskHandler struct {
	taskService      *services.TaskService
	cloudService     *services.CloudService
	accountService   *services.AccountService
	operationService *services.OperationService
	taskQueue        queue.ReliableTaskQueue
}

func NewTaskHandler(taskService *services.TaskService, cloudService *services.CloudService, accountService *services.AccountService) *TaskHandler {
	return &TaskHandler{
		taskService:    taskService,
		cloudService:   cloudService,
		accountService: accountService,
	}
}

// SetTaskQueue injects the configured queue backend for the legacy status
// helper. New routes use QueueStatusHandler directly; keeping both paths on
// the same abstraction prevents accidental Redis List status reads.
func (h *TaskHandler) SetTaskQueue(taskQueue queue.ReliableTaskQueue) {
	h.taskQueue = taskQueue
}

// TaskLogsResponse 任务日志响应
type TaskLogsResponse struct {
	TaskLogs []*dto.TaskLogResponse `json:"task_logs"`
	Total    int64                  `json:"total"`
	Page     int                    `json:"page"`
	PageSize int                    `json:"page_size"`
}

// GetTaskLogs 获取任务日志
// @Summary 获取任务日志
// @Tags 任务管理
// @Accept json
// @Produce json
// @Param account_id query int false "账号ID"
// @Param task_type query string false "任务类型"
// @Param status query string false "状态"
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

	taskType := c.Query("task_type")
	status := c.Query("status")

	taskLogs, total, err := h.taskService.GetTaskLogsContext(c.Request.Context(), userID.(uint), accountID, taskType, status, page, pageSize)
	if err != nil {
		respondTaskHandlerError(c, err)
		return
	}

	response.Success(c, TaskLogsResponse{
		TaskLogs: dto.ToTaskLogResponses(taskLogs),
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

	dashboard, err := h.cloudService.GetDashboardContext(c.Request.Context(), userID.(uint))
	if err != nil {
		respondInternalServer(c)
		return
	}

	response.Success(c, dashboard)
}

// CloudStatsResponse 云朵统计响应
type CloudStatsResponse struct {
	CloudStats []*dto.CloudStatsResponse `json:"cloud_stats"`
	Total      int64                     `json:"total"`
	Page       int                       `json:"page"`
	PageSize   int                       `json:"page_size"`
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
		cloudStats, total, err = h.cloudService.GetCloudStatsByAccountContext(c.Request.Context(), userID.(uint), uint(accountID), page, pageSize)
	} else {
		// 获取用户的所有统计
		cloudStats, total, err = h.cloudService.GetCloudStatsByUserIDContext(c.Request.Context(), userID.(uint), page, pageSize)
	}

	if err != nil {
		respondTaskHandlerError(c, err)
		return
	}

	response.Success(c, CloudStatsResponse{
		CloudStats: dto.ToCloudStatsResponses(cloudStats),
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
		trendData, err = h.cloudService.GetGlobalTrendDataContext(c.Request.Context(), days)
	} else {
		trendData, err = h.cloudService.GetTrendDataContext(c.Request.Context(), userID.(uint), days)
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
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	accounts, err := h.accountService.GetActiveAccountsContext(c.Request.Context(), userID)
	if err != nil {
		_ = c.Error(err)
		respondInternalServer(c)
		return
	}

	accountIDs := make([]uint, 0, len(accounts))
	dailyTaskTypes := h.taskService.DailyTaskTypes()
	for _, account := range accounts {
		if account == nil {
			continue
		}
		executed, err := h.taskService.HasExecutedTodayForTaskTypesContext(c.Request.Context(), account.ID, dailyTaskTypes)
		if err != nil {
			respondTaskHandlerError(c, err)
			return
		}
		if !executed {
			accountIDs = append(accountIDs, account.ID)
		}
	}
	if len(accountIDs) == 0 {
		response.Message(c, "所有账号今日已执行过任务")
		return
	}
	if h.operationService == nil {
		respondInternalServer(c)
		return
	}

	idempotencyKey, err := requestIdempotencyKey(c, fmt.Sprintf("account-task-batch:%d:%s", userID, time.Now().Format("2006-01-02")))
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	operation, _, dispatchErr, err := h.operationService.Submit(c.Request.Context(), services.SubmitOperationRequest{
		UserID:         userID,
		OperationType:  models.OperationTypeAccountTaskBatch,
		Payload:        services.AccountTaskBatchOperationPayload{AccountIDs: accountIDs},
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		respondOperationSubmitError(c, err)
		return
	}
	respondOperationAccepted(c, operation, dispatchErr)
}

// CalculateStats 手动计算统计数据// @Summary 手动计算统计数据
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
		if err := h.cloudService.CalculateDailyStatsContext(c.Request.Context()); err != nil {
			respondInternalServer(c)
			return
		}
	} else {
		// 普通用户仅计算自己的每日统计，避免触发全站账号重算。
		if err := h.cloudService.CalculateDailyStatsByUserIDContext(c.Request.Context(), userID.(uint)); err != nil {
			respondInternalServer(c)
			return
		}

		// 更新差异值
		if err := h.cloudService.UpdateCloudDiffsContext(c.Request.Context(), userID.(uint)); err != nil {
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

	total, err := h.cloudService.GetTotalCloudCountContext(c.Request.Context(), userID.(uint))
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
	metadata := queue.MetadataOf(h.taskQueue)
	errors := make([]string, 0)
	if h.taskQueue == nil {
		errors = append(errors, "reliable task queue is not configured")
	} else {
		var err error
		if queueLength, err = h.taskQueue.GetQueueLength(); err != nil {
			errors = append(errors, fmt.Sprintf("pending queue: %v", err))
		}
		if processingLength, err = h.taskQueue.GetProcessingLength(); err != nil {
			errors = append(errors, fmt.Sprintf("processing queue: %v", err))
		}
		if delayedLength, err = h.taskQueue.GetDelayedLength(); err != nil {
			errors = append(errors, fmt.Sprintf("delayed queue: %v", err))
		}
		if deadLetterLength, err = h.taskQueue.GetDeadLetterLength(); err != nil {
			errors = append(errors, fmt.Sprintf("dead-letter queue: %v", err))
		}
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
		Backend:         metadata.Backend,
		BackendMeta:     metadata,
		IsHealthy:       len(errors) == 0,
		Errors:          errors,
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
	logs, _, err := h.taskService.GetTaskLogsContext(c.Request.Context(), userID.(uint), nil, "", "", 1, 20)
	if err != nil {
		respondTaskHandlerError(c, err)
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

func (h *TaskHandler) SetOperationService(service *services.OperationService) {
	h.operationService = service
}
func respondTaskHandlerError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrAccountNotFound):
		respondBusinessError(c, http.StatusNotFound, appErrors.BusinessCodeAccountNotFound, "账号不存在")
	case errors.Is(err, context.DeadlineExceeded):
		respondBusinessError(c, http.StatusGatewayTimeout, appErrors.BusinessCodeTaskTimeout, "请求处理超时")
	case errors.Is(err, context.Canceled):
		respondBusinessError(c, http.StatusRequestTimeout, appErrors.BusinessCodeTaskTimeout, "请求已取消")
	default:
		if err != nil {
			_ = c.Error(err)
		}
		respondInternalServer(c)
	}
}
