package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"caiyun/internal/services"
	apiresponse "caiyun/pkg/response"
	"github.com/gin-gonic/gin"
)

func (h *ExchangeHandler) GetPendingPrizes(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	accountID, err := strconv.ParseUint(c.Query("account_id"), 10, 32)
	if err != nil || accountID == 0 {
		respondError(c, http.StatusBadRequest, "请选择云盘账号")
		return
	}
	if h.prizeService == nil {
		respondError(c, http.StatusServiceUnavailable, "领奖服务暂不可用")
		return
	}
	role, _ := c.Get("role")
	result, err := h.prizeService.GetPendingPrizes(c.Request.Context(), userID, uint(accountID), role == "admin", c.Query("refresh") == "1")
	if err != nil {
		if errors.Is(err, services.ErrAccountNotFound) {
			respondError(c, http.StatusNotFound, "云盘账号不存在")
		} else {
			respondExchangeServiceError(c, err)
		}
		return
	}
	apiresponse.Success(c, result)
}
