package handlers

import (
	"caiyun/internal/models"
	"caiyun/internal/services"
	apiresponse "caiyun/pkg/response"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

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

// UpdateExchangeConfig 更新抢兑配置（管理员）
func (h *ExchangeHandler) UpdateExchangeConfig(c *gin.Context) {
	var req UpdateExchangeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if req.Concurrency <= 0 || req.Concurrency > 1000 {
		respondError(c, http.StatusBadRequest, "并发数必须在 1 到 1000 之间")
		return
	}

	exchangeTime := ""
	if strings.TrimSpace(req.ExchangeTime) != "" {
		normalized, err := normalizeExchangeTime(req.ExchangeTime, "")
		if err != nil {
			respondError(c, http.StatusBadRequest, "自动兑换月卡时间"+err.Error())
			return
		}
		exchangeTime = normalized[:5]
	}

	monthlyPrizeID := strings.TrimSpace(req.MonthlyPrizeID)
	if len(monthlyPrizeID) > 100 {
		respondError(c, http.StatusBadRequest, "月卡商品ID长度不能超过 100")
		return
	}

	updates := []services.SystemConfigUpdate{
		{Key: "exchange_auto_update_products", Value: fmt.Sprintf("%v", req.AutoUpdateProducts), Description: "是否自动更新商品列表"},
		{Key: "exchange_concurrency", Value: fmt.Sprintf("%d", req.Concurrency), Description: "抢兑任务并发数量"},
		{Key: "exchange_enabled", Value: fmt.Sprintf("%v", req.Enabled), Description: "是否启用抢兑功能"},
		{Key: "exchange_monthly_enabled", Value: fmt.Sprintf("%v", req.ExchangeMonthlyEnabled), Description: "是否启用自动兑换月卡"},
		{Key: "exchange_immediate_enabled", Value: fmt.Sprintf("%v", req.ImmediateExchangeEnabled), Description: "是否启用立即兑换功能"},
	}
	if exchangeTime != "" {
		updates = append(updates, services.SystemConfigUpdate{Key: "exchange_monthly_time", Value: exchangeTime, Description: "自动兑换月卡时间"})
	}
	if monthlyPrizeID != "" {
		updates = append(updates, services.SystemConfigUpdate{Key: "exchange_monthly_prize_id", Value: monthlyPrizeID, Description: "月卡商品ID"})
	}

	if err := h.exchangeService.SetSystemConfigs(updates); err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "更新成功")
}

// ExecuteMonthlyExchange 立即执行兑换月卡（管理员）

// ExecuteMonthlyExchange 立即执行兑换月卡（管理员）
func (h *ExchangeHandler) ExecuteMonthlyExchange(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
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
		OperationType:  models.OperationTypeExchangeMonthly,
		Payload:        struct{}{},
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		respondOperationSubmitError(c, err)
		return
	}
	respondOperationAccepted(c, operation, dispatchErr)
}

// GetExchangeRecordsResponse 获取抢兑记录响应
