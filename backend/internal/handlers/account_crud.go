package handlers

import (
	"log"
	"net/http"
	"strconv"

	"caiyun/internal/dto"
	"caiyun/internal/services"
	"caiyun/internal/utils"
	apiresponse "caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

func (h *AccountHandler) CreateAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	var req CreateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	account, err := h.accountService.CreateAccountContext(c.Request.Context(), userID, &req)
	if err != nil {
		if err == services.ErrInvalidPhone {
			respondError(c, http.StatusBadRequest, "手机号格式不正确")
			return
		}
		if err == services.ErrAccountExists {
			respondError(c, http.StatusConflict, "账号已存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, dto.ToAccountResponse(account))
}

// ListAccounts 获取账号列表
// @Summary 获取账号列表
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param phone query string false "手机号搜索"
// @Success 200 {object} ListAccountsResponse
// @Failure 401 {object} ErrorResponse
// @Router /api/accounts [get]
func (h *AccountHandler) ListAccounts(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	phone := c.Query("phone")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	accounts, total, err := h.accountService.ListAccountsContext(c.Request.Context(), userID, page, pageSize, phone)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, ListAccountsResponse{
		Accounts: dto.ToAccountResponses(accounts),
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// GetAccount 获取账号详情
// @Summary 获取账号详情
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Success 200 {object} dto.AccountResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id} [get]
func (h *AccountHandler) GetAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	account, err := h.accountService.GetAccountContext(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, dto.ToAccountResponse(account))
}

// UpdateAccount 更新账号
// @Summary 更新账号
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Param request body UpdateAccountRequest true "更新账号请求"
// @Success 200 {object} dto.AccountResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id} [put]
func (h *AccountHandler) UpdateAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	var req UpdateAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	account, err := h.accountService.UpdateAccountContext(c.Request.Context(), userID, uint(accountID), &req)
	if err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		if err == services.ErrInvalidPhone {
			respondError(c, http.StatusBadRequest, "手机号格式不正确")
			return
		}
		if err == services.ErrAccountExists {
			respondError(c, http.StatusConflict, "账号已存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, dto.ToAccountResponse(account))
}

// DeleteAccount 删除账号
// @Summary 删除账号
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Success 200 {object} SuccessResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id} [delete]
func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	if err := h.accountService.DeleteAccountContext(c.Request.Context(), userID, uint(accountID)); err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "删除成功")
}

// SetAccountStatus 设置账号状态
// @Summary 设置账号状态
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Param is_active query bool true "是否激活"
// @Success 200 {object} SuccessResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id}/status [put]
func (h *AccountHandler) SetAccountStatus(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	// 获取状态，拒绝缺失或无法解析的值，避免默认把拼写错误当成停用。
	rawStatus := c.Query("is_active")
	if rawStatus == "" {
		respondError(c, http.StatusBadRequest, "is_active 参数不能为空")
		return
	}
	isActive, err := strconv.ParseBool(rawStatus)
	if err != nil {
		respondError(c, http.StatusBadRequest, "is_active 参数必须为 true 或 false")
		return
	}

	if err := h.accountService.SetAccountStatusContext(c.Request.Context(), userID, uint(accountID), isActive); err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "状态更新成功")
}

// RefreshToken 刷新账号Token
// @Summary 刷新账号Token
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Success 200 {object} dto.AccountResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id}/refresh [post]
func (h *AccountHandler) RefreshToken(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}

	// 获取账号ID
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	// 获取账号
	account, err := h.accountService.GetAccountContext(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		if err == services.ErrAccountNotFound {
			respondError(c, http.StatusNotFound, "账号不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	// 刷新Token
	if err := h.accountService.RefreshTokenContext(c.Request.Context(), account); err != nil {
		// 服务端打日志便于排查 500
		log.Printf("[RefreshToken] account_id=%d phone=%s err=%v", accountID, utils.MaskPhone(account.Phone), err)
		respondInternalServer(c)
		return
	}

	// 重新获取更新后的账号信息
	updatedAccount, err := h.accountService.GetAccountContext(c.Request.Context(), userID, uint(accountID))
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, dto.ToAccountResponse(updatedAccount))
}

// TriggerTask 手动触发任务执行
// @Summary 手动触发任务执行
// @Tags 账号管理
// @Accept json
// @Produce json
// @Param id path int true "账号ID"
// @Success 200 {object} SuccessResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /api/accounts/{id}/trigger [post]
