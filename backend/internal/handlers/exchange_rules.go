package handlers

import (
	"caiyun/internal/dto"
	apiresponse "caiyun/pkg/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type AddExchangeAccountRequest struct {
	AccountID     uint   `json:"account_id" binding:"required"`
	ProductID     *uint  `json:"product_id,omitempty"`
	Remark        string `json:"remark"`
	ExchangeTime1 string `json:"exchange_time_1"`
	ExchangeTime2 string `json:"exchange_time_2"`
}

// AddExchangeAccount 添加抢兑规则（历史命名保留）
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

	account, err := h.exchangeService.AddExchangeAccountContext(
		c.Request.Context(),
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

	apiresponse.Success(c, gin.H{
		"account": dto.ToExchangeRuleResponse(account),
		"rule":    dto.ToExchangeRuleResponse(account),
	})
}

// ExchangeAccountWithProduct 兑换账号及当前商品信息
type ExchangeAccountWithProduct struct {
	*dto.ExchangeRuleResponse
	CurrentProduct *dto.ProductResponse `json:"current_product,omitempty"`
}

// GetExchangeAccountsResponse 获取账号规则列表响应。accounts 为兼容旧前端保留，rules 是新语义字段。
type GetExchangeAccountsResponse struct {
	Accounts []*ExchangeAccountWithProduct `json:"accounts"`
	Rules    []*ExchangeAccountWithProduct `json:"rules,omitempty"`
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

	accounts, err := h.exchangeService.GetExchangeAccountsContext(c.Request.Context(), userID, isAdmin)
	if err != nil {
		respondInternalServer(c)
		return
	}

	// 为每个账号添加当前商品信息
	var accountsWithProduct []*ExchangeAccountWithProduct
	for _, acc := range accounts {
		accWithProd := &ExchangeAccountWithProduct{
			ExchangeRuleResponse: dto.ToExchangeRuleResponse(acc),
		}
		// 查找该账号的待执行或进行中的任务，获取商品信息
		for _, task := range acc.Tasks {
			if (task.Status == "pending" || task.Status == "running") && task.Product.ID > 0 {
				accWithProd.CurrentProduct = dto.ToProductResponse(&task.Product)
				break
			}
		}
		accountsWithProduct = append(accountsWithProduct, accWithProd)
	}

	apiresponse.Success(c, GetExchangeAccountsResponse{
		Accounts: accountsWithProduct,
		Rules:    accountsWithProduct,
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

	err = h.exchangeService.UpdateExchangeAccountContext(
		c.Request.Context(),
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

	err = h.exchangeService.DeleteExchangeAccountContext(c.Request.Context(), uint(id), userID)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	apiresponse.Message(c, "删除成功")
}

// AddExchangeRule 是 AddExchangeAccount 的新语义别名，旧 /accounts 路由仍保留。
func (h *ExchangeHandler) AddExchangeRule(c *gin.Context) { h.AddExchangeAccount(c) }

// GetExchangeRules 是 GetExchangeAccounts 的新语义别名。
func (h *ExchangeHandler) GetExchangeRules(c *gin.Context) { h.GetExchangeAccounts(c) }

// UpdateExchangeRule 是 UpdateExchangeAccount 的新语义别名。
func (h *ExchangeHandler) UpdateExchangeRule(c *gin.Context) { h.UpdateExchangeAccount(c) }

// DeleteExchangeRule 是 DeleteExchangeAccount 的新语义别名。
func (h *ExchangeHandler) DeleteExchangeRule(c *gin.Context) { h.DeleteExchangeAccount(c) }
