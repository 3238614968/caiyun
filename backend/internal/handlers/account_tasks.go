package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/services"

	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) TriggerTask(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	// Authorization and the daily policy are checked before accepting the
	// operation. Token refresh and all upstream business calls run in Worker.
	account, err := h.accountService.GetAccount(userID, uint(accountID))
	if err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		_ = c.Error(err)
		respondInternalServer(c)
		return
	}
	if h.taskService.HasExecutedToday(account.ID) {
		respondError(c, http.StatusTooManyRequests, "该账号今日已执行过任务，每天限手动执行一次")
		return
	}
	if h.operationService == nil {
		respondInternalServer(c)
		return
	}

	idempotencyKey, err := requestIdempotencyKey(c, fmt.Sprintf("account-task:%d:%s", account.ID, time.Now().Format("2006-01-02")))
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	operation, _, dispatchErr, err := h.operationService.Submit(c.Request.Context(), services.SubmitOperationRequest{
		UserID:         userID,
		OperationType:  models.OperationTypeAccountTask,
		AccountID:      account.ID,
		Payload:        services.AccountTaskOperationPayload{AccountID: account.ID, TaskType: "all_tasks"},
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		respondOperationSubmitError(c, err)
		return
	}
	respondOperationAccepted(c, operation, dispatchErr)
}
