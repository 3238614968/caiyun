package handlers

import (
	"caiyun/internal/models"
	"caiyun/internal/services"
	apiresponse "caiyun/pkg/response"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
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
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, SearchProductsResponse{
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
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, GetCategoriesResponse{
		Categories: categories,
	})
}

// UpdateProductsRequest 更新商品请求
type UpdateProductsRequest struct {
	AccountID uint `json:"account_id"`
}

// UpdateProducts 手动更新商品
func (h *ExchangeHandler) UpdateProducts(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req UpdateProductsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 验证账号 ID
	if req.AccountID == 0 {
		respondError(c, http.StatusBadRequest, "账号 ID 不能为空")
		return
	}

	role, _ := c.Get("role")
	isAdmin := role == "admin"

	// 调用 Service 更新商品（带账号 ID）
	count, err := h.productService.UpdateProducts(req.AccountID, userID, isAdmin)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.SuccessWithMessage(c, fmt.Sprintf("成功更新 %d 个商品", count), map[string]interface{}{
		"account_id": req.AccountID,
		"count":      count,
	})
}

// AddExchangeAccountRequest 添加兑换账号请求
type AddExchangeAccountRequest struct {
	AccountID     uint   `json:"account_id" binding:"required"`
	ProductID     *uint  `json:"product_id,omitempty"`
	Remark        string `json:"remark"`
	ExchangeTime1 string `json:"exchange_time_1"`
	ExchangeTime2 string `json:"exchange_time_2"`
}

// AddExchangeAccount 添加兑换账号
func (h *ExchangeHandler) AddExchangeAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req AddExchangeAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 设置默认时间
	exchangeTime1, err := normalizeExchangeTime(req.ExchangeTime1, "10:00:00")
	if err != nil {
		respondError(c, http.StatusBadRequest, "第一次抢兑时间"+err.Error())
		return
	}
	exchangeTime2, err := normalizeExchangeTime(req.ExchangeTime2, "16:00:00")
	if err != nil {
		respondError(c, http.StatusBadRequest, "第二次抢兑时间"+err.Error())
		return
	}

	account, err := h.exchangeService.AddExchangeAccount(
		userID,
		req.AccountID,
		req.Remark,
		exchangeTime1,
		exchangeTime2,
		req.ProductID,
	)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	apiresponse.Success(c, gin.H{"account": account})
}

// ExchangeAccountWithProduct 兑换账号及当前商品信息
type ExchangeAccountWithProduct struct {
	*models.ExchangeAccount
	CurrentProduct *models.Product `json:"current_product,omitempty"`
}

// GetExchangeAccountsResponse 获取兑换账号列表响应
type GetExchangeAccountsResponse struct {
	Accounts []*ExchangeAccountWithProduct `json:"accounts"`
	Total    int                           `json:"total"`
}

// GetExchangeAccounts 获取用户的兑换账号列表
func (h *ExchangeHandler) GetExchangeAccounts(c *gin.Context) {
	h.getExchangeAccounts(c, false)
}

// GetAdminExchangeAccounts 获取全站兑换账号列表（管理员路由专用）。
func (h *ExchangeHandler) GetAdminExchangeAccounts(c *gin.Context) {
	h.getExchangeAccounts(c, true)
}

func (h *ExchangeHandler) getExchangeAccounts(c *gin.Context, isAdmin bool) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	accounts, err := h.exchangeService.GetExchangeAccounts(userID, isAdmin)
	if err != nil {
		respondInternalServer(c)
		return
	}

	// 为每个账号添加当前商品信息
	var accountsWithProduct []*ExchangeAccountWithProduct
	for _, acc := range accounts {
		accWithProd := &ExchangeAccountWithProduct{
			ExchangeAccount: acc,
		}
		// 查找该账号的待执行或进行中的任务，获取商品信息
		for _, task := range acc.Tasks {
			if (task.Status == "pending" || task.Status == "running") && task.Product.ID > 0 {
				accWithProd.CurrentProduct = &task.Product
				break
			}
		}
		accountsWithProduct = append(accountsWithProduct, accWithProd)
	}

	apiresponse.Success(c, GetExchangeAccountsResponse{
		Accounts: accountsWithProduct,
		Total:    len(accountsWithProduct),
	})
}

// UpdateExchangeAccountRequest 更新兑换账号请求
type UpdateExchangeAccountRequest struct {
	Remark        string `json:"remark"`
	ExchangeTime1 string `json:"exchange_time_1"`
	ExchangeTime2 string `json:"exchange_time_2"`
	IsActive      bool   `json:"is_active"`
	ProductID     *uint  `json:"product_id,omitempty"` // 可选：修改要抢兑的商品
}

// UpdateExchangeAccount 更新兑换账号配置
func (h *ExchangeHandler) UpdateExchangeAccount(c *gin.Context) {
	h.updateExchangeAccount(c, false)
}

// UpdateAdminExchangeAccount 更新任意兑换账号配置（管理员路由专用）。
func (h *ExchangeHandler) UpdateAdminExchangeAccount(c *gin.Context) {
	h.updateExchangeAccount(c, true)
}

func (h *ExchangeHandler) updateExchangeAccount(c *gin.Context, isAdmin bool) {
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

	var req UpdateExchangeAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	exchangeTime1, err := normalizeExchangeTime(req.ExchangeTime1, "10:00:00")
	if err != nil {
		respondError(c, http.StatusBadRequest, "第一次抢兑时间"+err.Error())
		return
	}
	exchangeTime2, err := normalizeExchangeTime(req.ExchangeTime2, "16:00:00")
	if err != nil {
		respondError(c, http.StatusBadRequest, "第二次抢兑时间"+err.Error())
		return
	}

	err = h.exchangeService.UpdateExchangeAccount(
		uint(id),
		userID,
		isAdmin,
		req.Remark,
		exchangeTime1,
		exchangeTime2,
		req.IsActive,
		req.ProductID,
	)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	apiresponse.Message(c, "更新成功")
}

// DeleteExchangeAccount 删除兑换账号
func (h *ExchangeHandler) DeleteExchangeAccount(c *gin.Context) {
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

	err = h.exchangeService.DeleteExchangeAccount(uint(id), userID)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	apiresponse.Message(c, "删除成功")
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

	task, err := h.exchangeService.CreateExchangeTask(
		userID,
		req.ExchangeAccountID,
		req.ProductID,
		taskType,
		maxAttempts,
	)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	apiresponse.Success(c, gin.H{"task": task})
}

// GetExchangeTasksResponse 获取抢兑任务列表响应
type GetExchangeTasksResponse struct {
	Tasks []*models.ExchangeTask `json:"tasks"`
	Total int                    `json:"total"`
}

// GetExchangeTasks 获取用户的抢兑任务列表
func (h *ExchangeHandler) GetExchangeTasks(c *gin.Context) {
	h.getExchangeTasks(c, false)
}

// GetAdminExchangeTasks 获取全站抢兑任务列表（管理员路由专用）。
func (h *ExchangeHandler) GetAdminExchangeTasks(c *gin.Context) {
	h.getExchangeTasks(c, true)
}

func (h *ExchangeHandler) getExchangeTasks(c *gin.Context, isAdmin bool) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	tasks, err := h.exchangeService.GetExchangeTasks(userID, isAdmin)
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
type UpdateExchangeTaskRequest struct {
	MaxAttempts int `json:"max_attempts"`
}

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
func (h *ExchangeHandler) ExecuteExchangeTask(c *gin.Context) {
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

	err = h.exchangeService.ExecuteExchangeTask(uint(id), userID)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	apiresponse.Message(c, "已开始执行抢兑任务")
}

// BatchExecuteExchangeTasksRequest 批量执行抢兑任务请求
type BatchExecuteExchangeTasksRequest struct {
	TaskIDs []uint `json:"task_ids" binding:"required,min=1,max=50"`
}

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

	// 批量执行任务
	results := h.exchangeService.BatchExecuteExchangeTasks(req.TaskIDs, userID)

	apiresponse.Success(c, gin.H{
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
	ImmediateExchangeEnabled bool   `json:"immediate_exchange_enabled"`
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

	// 获取立即兑换开关
	immediateExchangeEnabled := false
	if config, err := h.exchangeService.GetSystemConfig("exchange_immediate_enabled"); err == nil {
		immediateExchangeEnabled = config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
	}

	apiresponse.Success(c, GetExchangeConfigResponse{
		AutoUpdateProducts:       autoUpdate,
		Concurrency:              concurrency,
		Enabled:                  enabled,
		ExchangeMonthlyEnabled:   exchangeMonthlyEnabled,
		ExchangeTime:             exchangeTime,
		MonthlyPrizeID:           monthlyPrizeID,
		ImmediateExchangeEnabled: immediateExchangeEnabled,
	})
}

// GetExchangeConfigPublic 获取抢兑配置（公开，普通用户可访问）
func (h *ExchangeHandler) GetExchangeConfigPublic(c *gin.Context) {
	// 只返回普通用户需要的配置

	// 获取启用状态
	enabled := true
	if config, err := h.exchangeService.GetSystemConfig("exchange_enabled"); err == nil {
		enabled = config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
	}

	// 获取立即兑换开关
	immediateExchangeEnabled := false
	if config, err := h.exchangeService.GetSystemConfig("exchange_immediate_enabled"); err == nil {
		immediateExchangeEnabled = config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
	}

	apiresponse.Success(c, gin.H{
		"enabled":                    enabled,
		"immediate_exchange_enabled": immediateExchangeEnabled,
	})
}

// UpdateExchangeConfigRequest 更新抢兑配置请求
type UpdateExchangeConfigRequest struct {
	AutoUpdateProducts       bool   `json:"auto_update_products"`
	Concurrency              int    `json:"concurrency"`
	Enabled                  bool   `json:"enabled"`
	ExchangeMonthlyEnabled   bool   `json:"exchange_monthly_enabled"`
	ExchangeTime             string `json:"exchange_time"`
	MonthlyPrizeID           string `json:"monthly_prize_id"`
	ImmediateExchangeEnabled bool   `json:"immediate_exchange_enabled"`
}

// UpdateExchangeConfig 更新抢兑配置（管理员）
func (h *ExchangeHandler) UpdateExchangeConfig(c *gin.Context) {
	var req UpdateExchangeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 更新自动更新配置
	if err := h.exchangeService.SetSystemConfig("exchange_auto_update_products", fmt.Sprintf("%v", req.AutoUpdateProducts), "是否自动更新商品列表"); err != nil {
		respondInternalServer(c)
		return
	}

	if req.Concurrency <= 0 || req.Concurrency > 1000 {
		respondError(c, http.StatusBadRequest, "并发数必须在 1 到 1000 之间")
		return
	}
	// 更新并发数配置
	if err := h.exchangeService.SetSystemConfig("exchange_concurrency", fmt.Sprintf("%d", req.Concurrency), "抢兑任务并发数量"); err != nil {
		respondInternalServer(c)
		return
	}

	// 更新启用状态
	if err := h.exchangeService.SetSystemConfig("exchange_enabled", fmt.Sprintf("%v", req.Enabled), "是否启用抢兑功能"); err != nil {
		respondInternalServer(c)
		return
	}

	// 更新兑换月卡开关
	if err := h.exchangeService.SetSystemConfig("exchange_monthly_enabled", fmt.Sprintf("%v", req.ExchangeMonthlyEnabled), "是否启用自动兑换月卡"); err != nil {
		respondInternalServer(c)
		return
	}

	// 更新兑换月卡时间
	if req.ExchangeTime != "" {
		exchangeTime, err := normalizeExchangeTime(req.ExchangeTime, "")
		if err != nil {
			respondError(c, http.StatusBadRequest, "自动兑换月卡时间"+err.Error())
			return
		}
		if err := h.exchangeService.SetSystemConfig("exchange_monthly_time", exchangeTime[:5], "自动兑换月卡时间"); err != nil {
			respondInternalServer(c)
			return
		}
	}

	// 更新月卡商品ID
	if req.MonthlyPrizeID != "" {
		if err := h.exchangeService.SetSystemConfig("exchange_monthly_prize_id", req.MonthlyPrizeID, "月卡商品ID"); err != nil {
			respondInternalServer(c)
			return
		}
	}

	// 更新立即兑换开关
	if err := h.exchangeService.SetSystemConfig("exchange_immediate_enabled", fmt.Sprintf("%v", req.ImmediateExchangeEnabled), "是否启用立即兑换功能"); err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "更新成功")
}

// ExecuteMonthlyExchange 立即执行兑换月卡（管理员）
func (h *ExchangeHandler) ExecuteMonthlyExchange(c *gin.Context) {
	// 异步执行月卡兑换，添加日志以便跟踪结果
	go func() {
		log.Println("[ExecuteMonthlyExchange] 开始执行月卡兑换任务")
		h.exchangeService.ExecuteMonthlyExchange()
		log.Println("[ExecuteMonthlyExchange] 月卡兑换任务执行完成")
	}()

	apiresponse.Message(c, "已开始执行月卡兑换任务")
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
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取筛选参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	page, limit = normalizePageLimit(page, limit, 20, 100)
	accountID, _ := strconv.ParseUint(c.Query("account_id"), 10, 32) // 查询参数，0 表示不筛选
	productName := c.Query("product_name")
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	records, total, err := h.exchangeService.GetExchangeRecords(
		userID,
		uint(accountID),
		productName,
		status,
		startDate,
		endDate,
		page,
		limit,
	)
	if err != nil {
		respondInternalServer(c)
		return
	}

	// 获取统计信息（最近 30 天）
	startTime := time.Now().AddDate(0, 0, -30)
	endTime := time.Now()
	successCount, failCount, err := h.exchangeService.GetRecordStats(userID, startTime, endTime)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, GetExchangeRecordsResponse{
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
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取筛选参数
	accountID, _ := strconv.ParseUint(c.Query("account_id"), 10, 32) // 查询参数，0 表示不筛选
	productName := c.Query("product_name")
	status := c.Query("status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	exportFormat := c.DefaultQuery("format", "csv") // csv 或 json

	// 获取所有记录（不分页）
	records, _, err := h.exchangeService.GetExchangeRecords(
		userID,
		uint(accountID),
		productName,
		status,
		startDate,
		endDate,
		1,
		10000, // 最多导出10000条
	)
	if err != nil {
		respondInternalServer(c)
		return
	}

	// 根据格式导出
	if exportFormat == "json" {
		// JSON格式
		c.Header("Content-Type", "application/json")
		c.Header("Content-Disposition", "attachment; filename=exchange_records.json")
		c.JSON(http.StatusOK, gin.H{
			"records":     records,
			"total":       len(records),
			"exported_at": time.Now().Format("2006-01-02 15:04:05"),
		})
		return
	}

	// CSV格式（默认）
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=exchange_records.csv")

	// 写入BOM以支持中文
	c.Writer.Write([]byte("\xEF\xBB\xBF"))

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	// 写入表头
	_ = writer.Write([]string{"记录ID", "用户ID", "账号ID", "商品ID", "商品名称", "状态", "消息", "执行时长(ms)", "创建时间"})

	// 写入数据
	for _, record := range records {
		_ = writer.Write([]string{
			strconv.FormatUint(uint64(record.ID), 10),
			strconv.FormatUint(uint64(record.UserID), 10),
			strconv.FormatUint(uint64(record.ExchangeAccountID), 10),
			strconv.FormatUint(uint64(record.ProductID), 10),
			escapeCSVFormula(record.PrizeName),
			escapeCSVFormula(record.Status),
			escapeCSVFormula(record.Message),
			strconv.Itoa(record.ExecutionTimeMs),
			record.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
}

func escapeCSVFormula(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed == "" {
		return value
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}

func normalizeExchangeTime(value, fallback string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}

	for _, layout := range []string{"15:04:05", "15:04"} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.Format("15:04:05"), nil
		}
	}
	return "", fmt.Errorf("格式错误，应为 HH:MM 或 HH:MM:SS")
}

func normalizeExchangeTaskType(taskType string) (string, error) {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		return string(models.ExchangeTaskFixed), nil
	}
	switch taskType {
	case string(models.ExchangeTaskFixed), string(models.ExchangeTaskLongTerm):
		return taskType, nil
	default:
		return "", fmt.Errorf("任务类型不合法")
	}
}

func normalizeMaxAttempts(maxAttempts int) (int, error) {
	if maxAttempts <= 0 {
		return 1, nil
	}
	if maxAttempts > 100 {
		return 0, fmt.Errorf("最大抢兑次数不能超过 100")
	}
	return maxAttempts, nil
}

func normalizePageLimit(page, limit, defaultLimit, maxLimit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit
}

// ImmediateExchangeRequest 立即兑换请求
type ImmediateExchangeRequest struct {
	ExchangeAccountID uint `json:"exchange_account_id" binding:"required"`
	ProductID         uint `json:"product_id" binding:"required"`
}

// ImmediateExchangeResponse 立即兑换响应
type ImmediateExchangeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ImmediateExchange 立即兑换（无需创建任务，直接执行）
func (h *ExchangeHandler) ImmediateExchange(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 检查是否启用了立即兑换功能
	immediateEnabled := false
	if config, err := h.exchangeService.GetSystemConfig("exchange_immediate_enabled"); err == nil {
		immediateEnabled = config.KeyValue == "true" || config.KeyValue == "1" || config.KeyValue == "yes"
	}

	if !immediateEnabled {
		respondError(c, http.StatusForbidden, "立即兑换功能未启用")
		return
	}

	var req ImmediateExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 创建临时任务并立即执行
	task, err := h.exchangeService.CreateExchangeTask(
		userID,
		req.ExchangeAccountID,
		req.ProductID,
		"immediate",
		1,
	)
	if err != nil {
		log.Printf("[ImmediateExchange] 创建立即兑换任务失败 user_id=%v exchange_account_id=%d product_id=%d: %v",
			userID, req.ExchangeAccountID, req.ProductID, err)
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	// 立即执行任务
	go h.exchangeService.ExecuteExchangeTask(task.ID, userID)

	apiresponse.Success(c, ImmediateExchangeResponse{
		Success: true,
		Message: "兑换任务已启动",
	})
}
