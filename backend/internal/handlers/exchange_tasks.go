package handlers

import (
	"caiyun/internal/models"
	"caiyun/internal/services"
	apiresponse "caiyun/pkg/response"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func uniqueUintValues(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func mergeUintAliasIDs(groups ...[]uint) []uint {
	merged := make([]uint, 0)
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return uniqueUintValues(merged)
}

// CreateExchangeTaskRequest 创建抢兑任务请求

// CreateExchangeTaskRequest 创建抢兑任务请求
type CreateExchangeTaskRequest struct {
	ExchangeRuleID     uint   `json:"exchange_rule_id"`
	ExchangeRuleIDs    []uint `json:"exchange_rule_ids"`
	ExchangeAccountID  uint   `json:"exchange_account_id"`
	ExchangeAccountIDs []uint `json:"exchange_account_ids"`
	AccountID          uint   `json:"account_id"`
	AccountIDs         []uint `json:"account_ids"`
	ProductID          uint   `json:"product_id" binding:"required"`
	TaskType           string `json:"task_type"` // fixed or long_term
	MaxAttempts        int    `json:"max_attempts"`
	// 预定/长期抢兑配置
	ScheduledExchangeTime string `json:"scheduled_exchange_time"`
	RestockCycle          string `json:"restock_cycle"`
	RestockWeekday        *int   `json:"restock_weekday"`
	RestockDayOfMonth     *int   `json:"restock_day_of_month"`
	RestockTimes          any    `json:"restock_times"`
	CustomCron            string `json:"custom_cron"`
	CalendarPolicy        string `json:"calendar_policy"`
	HolidayDates          any    `json:"holiday_dates"`
	WorkdayDates          any    `json:"workday_dates"`
}

func (req CreateExchangeTaskRequest) NormalizedExchangeRuleIDs() []uint {
	singleIDs := make([]uint, 0, 2)
	if req.ExchangeRuleID > 0 {
		singleIDs = append(singleIDs, req.ExchangeRuleID)
	}
	if req.ExchangeAccountID > 0 {
		singleIDs = append(singleIDs, req.ExchangeAccountID)
	}
	return mergeUintAliasIDs(req.ExchangeRuleIDs, req.ExchangeAccountIDs, singleIDs)
}

func (req CreateExchangeTaskRequest) NormalizedAccountIDs() []uint {
	singleIDs := make([]uint, 0, 1)
	if req.AccountID > 0 {
		singleIDs = append(singleIDs, req.AccountID)
	}
	return mergeUintAliasIDs(req.AccountIDs, singleIDs)
}

// CreateExchangeTask 创建抢兑任务

// CreateExchangeTask 创建抢兑任务
func (h *ExchangeHandler) CreateExchangeTask(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req CreateExchangeTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 设置默认值
	taskType, err := normalizeExchangeTaskType(req.TaskType)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	maxAttempts, err := normalizeMaxAttempts(req.MaxAttempts)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	scheduledExchangeTime, err := normalizeExchangeTime(req.ScheduledExchangeTime, "")
	if err != nil {
		respondError(c, http.StatusBadRequest, "指定抢兑时间"+err.Error())
		return
	}
	restockCycle, restockWeekday, restockDayOfMonth, err := normalizeRestockConfig(req.RestockCycle, req.RestockWeekday, req.RestockDayOfMonth)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	restockTimes, customCron, calendarPolicy, holidayDates, workdayDates, err := normalizeExchangeScheduleExtras(
		req.RestockTimes, req.CustomCron, req.CalendarPolicy, req.HolidayDates, req.WorkdayDates,
	)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	exchangeRuleIDs := req.NormalizedExchangeRuleIDs()
	accountIDs := req.NormalizedAccountIDs()
	if len(exchangeRuleIDs) == 0 && len(accountIDs) == 0 {
		respondError(c, http.StatusBadRequest, "请选择云盘账号或抢兑规则")
		return
	}

	result := h.exchangeService.CreateExchangeTasks(
		userID,
		exchangeRuleIDs,
		accountIDs,
		req.ProductID,
		services.ExchangeTaskCreateOptions{
			TaskType:              taskType,
			MaxAttempts:           maxAttempts,
			ScheduledExchangeTime: scheduledExchangeTime,
			RestockCycle:          restockCycle,
			RestockWeekday:        restockWeekday,
			RestockDayOfMonth:     restockDayOfMonth,
			RestockTimes:          restockTimes,
			CustomCron:            customCron,
			CalendarPolicy:        calendarPolicy,
			HolidayDates:          holidayDates,
			WorkdayDates:          workdayDates,
		},
	)
	if len(result.Tasks) == 0 {
		message := "创建抢兑任务失败"
		if len(result.Errors) > 0 {
			message = strings.Join(result.Errors, "；")
		}
		respondError(c, http.StatusBadRequest, message)
		return
	}

	apiresponse.Success(c, gin.H{
		"task":    result.Tasks[0],
		"tasks":   result.Tasks,
		"created": len(result.Tasks),
		"errors":  result.Errors,
		"results": result.Results,
	})
}

// GetExchangeTasksResponse 获取抢兑任务列表响应

// GetExchangeTasksResponse 获取抢兑任务列表响应
type GetExchangeTasksResponse struct {
	Tasks []*models.ExchangeTask `json:"tasks"`
	Total int                    `json:"total"`
}

// GetExchangeTasks 获取用户的抢兑任务列表

// GetExchangeTasks 获取用户的抢兑任务列表
func (h *ExchangeHandler) GetExchangeTasks(c *gin.Context) {
	h.getExchangeTasks(c, false)
}

// GetAdminExchangeTasks 获取全站抢兑任务列表（管理员路由专用）。

// GetAdminExchangeTasks 获取全站抢兑任务列表（管理员路由专用）。
func (h *ExchangeHandler) GetAdminExchangeTasks(c *gin.Context) {
	h.getExchangeTasks(c, true)
}

func (h *ExchangeHandler) getExchangeTasks(c *gin.Context, isAdmin bool) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	filter := parseExchangeTaskFilter(c)
	tasks, err := h.exchangeService.GetExchangeTasksWithFilter(userID, isAdmin, filter)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, GetExchangeTasksResponse{
		Tasks: tasks,
		Total: len(tasks),
	})
}

// UpdateExchangeTaskRequest 更新抢兑任务请求

// UpdateExchangeTaskRequest 更新抢兑任务请求
type UpdateExchangeTaskRequest struct {
	MaxAttempts int `json:"max_attempts"`
}

// UpdateExchangeTask 更新抢兑任务

// UpdateExchangeTask 更新抢兑任务
func (h *ExchangeHandler) UpdateExchangeTask(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的 ID")
		return
	}

	var req UpdateExchangeTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	maxAttempts, err := normalizeMaxAttempts(req.MaxAttempts)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	err = h.exchangeService.UpdateExchangeTask(uint(id), userID, maxAttempts)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	apiresponse.Message(c, "更新成功")
}

// DeleteExchangeTask 删除抢兑任务

// DeleteExchangeTask 删除抢兑任务
func (h *ExchangeHandler) DeleteExchangeTask(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的 ID")
		return
	}

	err = h.exchangeService.DeleteExchangeTask(uint(id), userID)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	apiresponse.Message(c, "删除成功")
}

// ExecuteExchangeTask 立即执行抢兑任务

// ExecuteExchangeTask 立即执行抢兑任务
func (h *ExchangeHandler) ExecuteExchangeTask(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的 ID")
		return
	}
	if h.operationService == nil {
		respondInternalServer(c)
		return
	}
	idempotencyKey, err := requestIdempotencyKey(c, "")
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	operation, _, dispatchErr, err := h.operationService.Submit(c.Request.Context(), services.SubmitOperationRequest{
		UserID:         userID,
		OperationType:  models.OperationTypeExchangeTask,
		ResourceID:     uint(id),
		Payload:        services.ExchangeTaskOperationPayload{TaskID: uint(id)},
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		respondOperationSubmitError(c, err)
		return
	}
	respondOperationAccepted(c, operation, dispatchErr)
}

// BatchExecuteExchangeTasksRequest 批量执行抢兑任务请求
// BatchExecuteExchangeTasksRequest 批量执行抢兑任务请求
type BatchExecuteExchangeTasksRequest struct {
	TaskIDs []uint `json:"task_ids" binding:"required,min=1,max=50"`
}

// BatchExecuteExchangeTasks 批量执行抢兑任务

// BatchExecuteExchangeTasks 批量执行抢兑任务
func (h *ExchangeHandler) BatchExecuteExchangeTasks(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req BatchExecuteExchangeTasksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	req.TaskIDs = uniqueUintValues(req.TaskIDs)
	if len(req.TaskIDs) == 0 {
		respondError(c, http.StatusBadRequest, "请选择需要执行的抢兑任务")
		return
	}
	if h.operationService == nil {
		respondInternalServer(c)
		return
	}
	idempotencyKey, err := requestIdempotencyKey(c, "")
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	operation, _, dispatchErr, err := h.operationService.Submit(c.Request.Context(), services.SubmitOperationRequest{
		UserID:         userID,
		OperationType:  models.OperationTypeExchangeTaskBatch,
		Payload:        services.ExchangeTaskBatchOperationPayload{TaskIDs: req.TaskIDs},
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		respondOperationSubmitError(c, err)
		return
	}
	respondOperationAccepted(c, operation, dispatchErr)
}

// GetExchangeConfigResponse 获取抢兑配置响应
