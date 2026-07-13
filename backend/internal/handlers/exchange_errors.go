package handlers

import (
	"errors"
	"log"
	"net/http"

	"caiyun/internal/services"
	appErrors "caiyun/pkg/errors"
	apiresponse "caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

// respondExchangeServiceError is the only service-error mapping used by
// exchange handlers. Unknown errors are logged with the request ID and returned
// as a generic 500; driver/upstream error strings never cross the HTTP boundary.
func respondExchangeServiceError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, services.ErrExchangeInvalidInput):
		respondError(c, http.StatusBadRequest, "请求参数错误")
	case errors.Is(err, services.ErrExchangeCloudAccountMissing):
		respondBusinessError(c, http.StatusNotFound, appErrors.BusinessCodeAccountNotFound, "云盘账号不存在")
	case errors.Is(err, services.ErrExchangeRuleNotFound):
		respondError(c, http.StatusNotFound, "账号规则不存在")
	case errors.Is(err, services.ErrExchangeTaskNotFound):
		respondError(c, http.StatusNotFound, "抢兑任务不存在")
	case errors.Is(err, services.ErrExchangeProductNotFound):
		respondBusinessError(c, http.StatusNotFound, appErrors.BusinessCodeExchangeProductNotFound, "商品不存在")
	case errors.Is(err, services.ErrExchangePermissionDenied):
		respondBusinessError(c, http.StatusForbidden, appErrors.BusinessCodeAuthPermissionDenied, "无权执行该操作")
	case errors.Is(err, services.ErrExchangeAccountDisabled):
		respondBusinessError(c, http.StatusConflict, appErrors.BusinessCodeAccountDisabled, "云盘账号未启用")
	case errors.Is(err, services.ErrExchangeCredentialsMissing):
		respondBusinessError(c, http.StatusConflict, appErrors.BusinessCodeAccountTokenExpired, "云盘账号鉴权信息缺失")
	case errors.Is(err, services.ErrExchangeProductInactive):
		respondBusinessError(c, http.StatusConflict, appErrors.BusinessCodeExchangeStockUnavailable, "商品已下架，无法抢兑")
	case errors.Is(err, services.ErrExchangeTaskConflict):
		respondBusinessError(c, http.StatusConflict, appErrors.BusinessCodeExchangeTaskConflict, "该账号已存在此商品的抢兑任务")
	default:
		requestID := c.GetString("request_id")
		log.Printf("[ExchangeHandler] request_id=%s internal_error=%v", requestID, err)
		apiresponse.Error(c, err)
	}
}
