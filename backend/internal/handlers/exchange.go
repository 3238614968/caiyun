package handlers

import (
	"caiyun/internal/models"
	"caiyun/internal/services"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ExchangeHandler struct {
	exchangeService *services.ExchangeService
	productService  *services.ProductService
}

func NewExchangeHandler(exchangeService *services.ExchangeService, productService *services.ProductService) *ExchangeHandler {
	return &ExchangeHandler{
		exchangeService: exchangeService,
		productService:  productService,
	}
}

// SearchProductsResponse 搜索商品响应
type SearchProductsResponse struct {
	Products []*models.Product `json:"products"`
	Total    int64             `json:"total"`
}

// SearchProducts 搜索商品
func (h *ExchangeHandler) SearchProducts(c *gin.Context) {
	keyword := c.Query("keyword")
	limitStr := c.DefaultQuery("limit", "20")
	
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 20
	}

	products, err := h.exchangeService.SearchProducts(keyword, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, SearchProductsResponse{
		Products: products,
		Total:    int64(len(products)),
	})
}

// GetCategoriesResponse 获取分类响应
type GetCategoriesResponse struct {
	Categories []string `json:"categories"`
}

// GetCategories 获取商品分类
func (h *ExchangeHandler) GetCategories(c *gin.Context) {
	categories, err := h.exchangeService.GetProductCategories()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetCategoriesResponse{
		Categories: categories,
	})
}

// UpdateProductsRequest 更新商品请求
type UpdateProductsRequest struct {
	AccountID uint `json:"account_id"`
}

// UpdateProducts 手动更新商品
func (h *ExchangeHandler) UpdateProducts(c *gin.Context) {
	var req UpdateProductsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	// 验证账号 ID
	if req.AccountID == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "账号 ID 不能为空"})
		return
	}

	// 调用 Service 更新商品（带账号 ID）
	count, err := h.productService.UpdateProducts(req.AccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: "更新失败：" + err.Error()})
		return
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"code":    0,
		"message": fmt.Sprintf("成功更新 %d 个商品", count),
		"data": map[string]interface{}{
			"account_id": req.AccountID,
			"count":      count,
		},
	})
}

// AddExchangeAccountRequest 添加兑换账号请求
type AddExchangeAccountRequest struct {
	AccountID     uint   `json:"account_id" binding:"required"`
	Remark        string `json:"remark"`
	ExchangeTime1 string `json:"exchange_time_1"`
	ExchangeTime2 string `json:"exchange_time_2"`
}

// AddExchangeAccount 添加兑换账号
func (h *ExchangeHandler) AddExchangeAccount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	var req AddExchangeAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	// 设置默认时间
	if req.ExchangeTime1 == "" {
		req.ExchangeTime1 = "10:00:00"
	}
	if req.ExchangeTime2 == "" {
		req.ExchangeTime2 = "16:00:00"
	}

	account, err := h.exchangeService.AddExchangeAccount(
		userID.(uint),
		req.AccountID,
		req.Remark,
		req.ExchangeTime1,
		req.ExchangeTime2,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"account": account})
}

// GetExchangeAccountsResponse 获取兑换账号列表响应
type GetExchangeAccountsResponse struct {
	Accounts []*models.ExchangeAccount `json:"accounts"`
	Total    int                       `json:"total"`
}

// GetExchangeAccounts 获取用户的兑换账号列表
func (h *ExchangeHandler) GetExchangeAccounts(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	accounts, err := h.exchangeService.GetExchangeAccounts(userID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetExchangeAccountsResponse{
		Accounts: accounts,
		Total:    len(accounts),
	})
}

// UpdateExchangeAccountRequest 更新兑换账号请求
type UpdateExchangeAccountRequest struct {
	Remark        string `json:"remark"`
	ExchangeTime1 string `json:"exchange_time_1"`
	ExchangeTime2 string `json:"exchange_time_2"`
	IsActive      bool   `json:"is_active"`
}

// UpdateExchangeAccount 更新兑换账号配置
func (h *ExchangeHandler) UpdateExchangeAccount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	idStr := c.Param("id")
	id, _ := strconv.ParseUint(idStr, 10, 32)

	var req UpdateExchangeAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	err := h.exchangeService.UpdateExchangeAccount(
		uint(id),
		userID.(uint),
		req.Remark,
		req.ExchangeTime1,
		req.ExchangeTime2,
		req.IsActive,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "更新成功"})
}

// DeleteExchangeAccount 删除兑换账号
func (h *ExchangeHandler) DeleteExchangeAccount(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	idStr := c.Param("id")
	id, _ := strconv.ParseUint(idStr, 10, 32)

	err := h.exchangeService.DeleteExchangeAccount(uint(id), userID.(uint))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "删除成功"})
}

// CreateExchangeTaskRequest 创建抢兑任务请求
type CreateExchangeTaskRequest struct {
	ExchangeAccountID uint   `json:"exchange_account_id" binding:"required"`
	ProductID         uint   `json:"product_id" binding:"required"`
	TaskType          string `json:"task_type"` // fixed or long_term
	MaxAttempts       int    `json:"max_attempts"`
}

// CreateExchangeTask 创建抢兑任务
func (h *ExchangeHandler) CreateExchangeTask(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	var req CreateExchangeTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	// 设置默认值
	if req.TaskType == "" {
		req.TaskType = "fixed"
	}
	if req.MaxAttempts <= 0 {
		req.MaxAttempts = 1
	}

	task, err := h.exchangeService.CreateExchangeTask(
		userID.(uint),
		req.ExchangeAccountID,
		req.ProductID,
		req.TaskType,
		req.MaxAttempts,
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"task": task})
}

// GetExchangeTasksResponse 获取抢兑任务列表响应
type GetExchangeTasksResponse struct {
	Tasks []*models.ExchangeTask `json:"tasks"`
	Total int                    `json:"total"`
}

// GetExchangeTasks 获取用户的抢兑任务列表
func (h *ExchangeHandler) GetExchangeTasks(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	tasks, err := h.exchangeService.GetExchangeTasks(userID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetExchangeTasksResponse{
		Tasks: tasks,
		Total: len(tasks),
	})
}

// UpdateExchangeTaskRequest 更新抢兑任务请求
type UpdateExchangeTaskRequest struct {
	MaxAttempts int `json:"max_attempts"`
}

// UpdateExchangeTask 更新抢兑任务
func (h *ExchangeHandler) UpdateExchangeTask(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	idStr := c.Param("id")
	id, _ := strconv.ParseUint(idStr, 10, 32)

	var req UpdateExchangeTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	err := h.exchangeService.UpdateExchangeTask(uint(id), userID.(uint), req.MaxAttempts)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "更新成功"})
}

// DeleteExchangeTask 删除抢兑任务
func (h *ExchangeHandler) DeleteExchangeTask(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	idStr := c.Param("id")
	id, _ := strconv.ParseUint(idStr, 10, 32)

	err := h.exchangeService.DeleteExchangeTask(uint(id), userID.(uint))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "删除成功"})
}

// ExecuteExchangeTask 立即执行抢兑任务
func (h *ExchangeHandler) ExecuteExchangeTask(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	idStr := c.Param("id")
	id, _ := strconv.ParseUint(idStr, 10, 32)

	err := h.exchangeService.ExecuteExchangeTask(uint(id), userID.(uint))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "已开始执行抢兑任务"})
}

// BatchExecuteExchangeTasksRequest 批量执行抢兑任务请求
type BatchExecuteExchangeTasksRequest struct {
	TaskIDs []uint `json:"task_ids" binding:"required,min=1,max=50"`
}

// BatchExecuteExchangeTasks 批量执行抢兑任务
func (h *ExchangeHandler) BatchExecuteExchangeTasks(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	var req BatchExecuteExchangeTasksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	// 批量执行任务
	results := h.exchangeService.BatchExecuteExchangeTasks(req.TaskIDs, userID.(uint))

	c.JSON(http.StatusOK, gin.H{
		"message": "批量执行完成",
		"results": results,
	})
}

// GetExchangeConfigResponse 获取抢兑配置响应
type GetExchangeConfigResponse struct {
	AutoUpdateProducts       bool   `json:"auto_update_products"`
	Concurrency              int    `json:"concurrency"`
	Enabled                  bool   `json:"enabled"`
	ExchangeMonthlyEnabled   bool   `json:"exchange_monthly_enabled"`
	ExchangeTime             string `json:"exchange_time"`
	MonthlyPrizeID           string `json:"monthly_prize_id"`
}

// GetExchangeConfig 获取抢兑配置（管理员）
func (h *ExchangeHandler) GetExchangeConfig(c *gin.Context) {
	// 获取自动更新配置
	autoUpdate := false
	if config, err := h.exchangeService.GetSystemConfig("exchange_auto_update_products"); err == nil {
		autoUpdate = config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
	}

	// 获取并发数配置（使用 strconv 代替 fmt.Sscanf）
	concurrency := 10
	if config, err := h.exchangeService.GetSystemConfig("exchange_concurrency"); err == nil && config.KeyValue != "" {
		if val, err := strconv.Atoi(config.KeyValue); err == nil && val > 0 {
			concurrency = val
		}
	}

	// 获取启用状态
	enabled := true
	if config, err := h.exchangeService.GetSystemConfig("exchange_enabled"); err == nil {
		enabled = config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
	}

	// 获取兑换月卡开关
	exchangeMonthlyEnabled := false
	if config, err := h.exchangeService.GetSystemConfig("exchange_monthly_enabled"); err == nil {
		exchangeMonthlyEnabled = config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
	}

	// 获取兑换月卡时间
	exchangeTime := "00:00"
	if config, err := h.exchangeService.GetSystemConfig("exchange_monthly_time"); err == nil && config.KeyValue != "" {
		exchangeTime = config.KeyValue
	}

	// 获取月卡商品ID
	monthlyPrizeID := "1001"
	if config, err := h.exchangeService.GetSystemConfig("exchange_monthly_prize_id"); err == nil && config.KeyValue != "" {
		monthlyPrizeID = config.KeyValue
	}

	c.JSON(http.StatusOK, GetExchangeConfigResponse{
		AutoUpdateProducts:     autoUpdate,
		Concurrency:            concurrency,
		Enabled:                enabled,
		ExchangeMonthlyEnabled: exchangeMonthlyEnabled,
		ExchangeTime:           exchangeTime,
		MonthlyPrizeID:         monthlyPrizeID,
	})
}

// UpdateExchangeConfigRequest 更新抢兑配置请求
type UpdateExchangeConfigRequest struct {
	AutoUpdateProducts     bool   `json:"auto_update_products"`
	Concurrency            int    `json:"concurrency"`
	Enabled                bool   `json:"enabled"`
	ExchangeMonthlyEnabled bool   `json:"exchange_monthly_enabled"`
	ExchangeTime           string `json:"exchange_time"`
	MonthlyPrizeID         string `json:"monthly_prize_id"`
}

// UpdateExchangeConfig 更新抢兑配置（管理员）
func (h *ExchangeHandler) UpdateExchangeConfig(c *gin.Context) {
	var req UpdateExchangeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: err.Error()})
		return
	}

	// 更新自动更新配置
	if err := h.exchangeService.SetSystemConfig("exchange_auto_update_products", fmt.Sprintf("%v", req.AutoUpdateProducts), "是否自动更新商品列表"); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	// 更新并发数配置
	if err := h.exchangeService.SetSystemConfig("exchange_concurrency", fmt.Sprintf("%d", req.Concurrency), "抢兑任务并发数量"); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	// 更新启用状态
	if err := h.exchangeService.SetSystemConfig("exchange_enabled", fmt.Sprintf("%v", req.Enabled), "是否启用抢兑功能"); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	// 更新兑换月卡开关
	if err := h.exchangeService.SetSystemConfig("exchange_monthly_enabled", fmt.Sprintf("%v", req.ExchangeMonthlyEnabled), "是否启用自动兑换月卡"); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	// 更新兑换月卡时间
	if req.ExchangeTime != "" {
		if err := h.exchangeService.SetSystemConfig("exchange_monthly_time", req.ExchangeTime, "自动兑换月卡时间"); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
			return
		}
	}

	// 更新月卡商品ID
	if req.MonthlyPrizeID != "" {
		if err := h.exchangeService.SetSystemConfig("exchange_monthly_prize_id", req.MonthlyPrizeID, "月卡商品ID"); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, SuccessResponse{Message: "更新成功"})
}

// ExecuteMonthlyExchange 立即执行兑换月卡（管理员）
func (h *ExchangeHandler) ExecuteMonthlyExchange(c *gin.Context) {
	// 异步执行月卡兑换
	go h.exchangeService.ExecuteMonthlyExchange()

	c.JSON(http.StatusOK, SuccessResponse{Message: "已开始执行月卡兑换任务"})
}

// GetExchangeRecordsResponse 获取抢兑记录响应
type GetExchangeRecordsResponse struct {
	Records []*models.ExchangeRecord `json:"records"`
	Total   int64                    `json:"total"`
	Stats   RecordStats              `json:"stats"`
}

type RecordStats struct {
	Success int64 `json:"success"`
	Failed  int64 `json:"failed"`
}

// GetExchangeRecords 获取抢兑记录列表
func (h *ExchangeHandler) GetExchangeRecords(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	// 获取筛选参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	accountID, _ := strconv.ParseUint(c.Query("account_id"), 10, 32)
	productName := c.Query("product_name")
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	records, total, err := h.exchangeService.GetExchangeRecords(
		userID.(uint),
		uint(accountID),
		productName,
		status,
		startDate,
		endDate,
		page,
		limit,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	// 获取统计信息（最近 30 天）
	startTime := time.Now().AddDate(0, 0, -30)
	endTime := time.Now()
	successCount, failCount, err := h.exchangeService.GetRecordStats(userID.(uint), startTime, endTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetExchangeRecordsResponse{
		Records: records,
		Total:   total,
		Stats: RecordStats{
			Success: successCount,
			Failed:  failCount,
		},
	})
}

// ExportExchangeRecords 导出抢兑记录
func (h *ExchangeHandler) ExportExchangeRecords(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Message: "未授权"})
		return
	}

	// 获取筛选参数
	accountID, _ := strconv.ParseUint(c.Query("account_id"), 10, 32)
	productName := c.Query("product_name")
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	exportFormat := c.DefaultQuery("format", "csv") // csv 或 json

	// 获取所有记录（不分页）
	records, _, err := h.exchangeService.GetExchangeRecords(
		userID.(uint),
		uint(accountID),
		productName,
		status,
		startDate,
		endDate,
		1,
		10000, // 最多导出10000条
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Message: err.Error()})
		return
	}

	// 根据格式导出
	if exportFormat == "json" {
		// JSON格式
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", "attachment; filename=exchange_records.json")
		c.JSON(http.StatusOK, gin.H{
			"records": records,
			"total":   len(records),
			"exported_at": time.Now().Format("2006-01-02 15:04:05"),
		})
		return
	}

	// CSV格式（默认）
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=exchange_records.csv")

	// 写入BOM以支持中文
	c.Writer.Write([]byte("\xEF\xBB\xBF"))

	// 写入表头
	c.Writer.Write([]byte("记录ID,用户ID,账号ID,商品ID,商品名称,状态,消息,执行时长(ms),创建时间\n"))

	// 写入数据
	for _, record := range records {
		line := fmt.Sprintf("%d,%d,%d,%d,%s,%s,%s,%d,%s\n",
			record.ID,
			record.UserID,
			record.ExchangeAccountID,
			record.ProductID,
			record.PrizeName,
			record.Status,
			record.Message,
			record.ExecutionTimeMs,
			record.CreatedAt.Format("2006-01-02 15:04:05"),
		)
		c.Writer.Write([]byte(line))
	}
}
