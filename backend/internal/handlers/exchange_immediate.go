package handlers

import (
	"net/http"

	"caiyun/internal/models"
	"caiyun/internal/services"

	"github.com/gin-gonic/gin"
)

// ImmediateExchangeRequest describes a durable immediate-exchange command.
type ImmediateExchangeRequest struct {
	ExchangeRuleID    uint `json:"exchange_rule_id"`
	ExchangeAccountID uint `json:"exchange_account_id"`
	AccountID         uint `json:"account_id"`
	ProductID         uint `json:"product_id" binding:"required"`
}

func (req ImmediateExchangeRequest) NormalizedExchangeRuleID() uint {
	if req.ExchangeRuleID > 0 {
		return req.ExchangeRuleID
	}
	return req.ExchangeAccountID
}

// ImmediateExchange validates feature policy and persists the command. Task
// creation and exchange execution are both performed by Worker, so an API Pod
// restart cannot orphan an in-process goroutine.
func (h *ExchangeHandler) ImmediateExchange(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

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
		respondError(c, http.StatusBadRequest, "请求参数错误")
		return
	}
	exchangeRuleID := req.NormalizedExchangeRuleID()
	if exchangeRuleID == 0 && req.AccountID == 0 {
		respondError(c, http.StatusBadRequest, "请选择云盘账号或抢兑规则")
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
		UserID:        userID,
		OperationType: models.OperationTypeExchangeImmediate,
		AccountID:     req.AccountID,
		Payload: services.ImmediateExchangeOperationPayload{
			ExchangeRuleID: exchangeRuleID,
			AccountID:      req.AccountID,
			ProductID:      req.ProductID,
		},
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		respondOperationSubmitError(c, err)
		return
	}
	respondOperationAccepted(c, operation, dispatchErr)
}
