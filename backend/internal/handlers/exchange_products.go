package handlers

import (
	"caiyun/internal/dto"
	apiresponse "caiyun/pkg/response"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// SearchProductsResponse 搜索商品响应
type SearchProductsResponse struct {
	Products []*dto.ProductResponse `json:"products"`
	Total    int64                  `json:"total"`
}

// SearchProducts 搜索商品
func (h *ExchangeHandler) SearchProducts(c *gin.Context) {
	keyword := c.Query("keyword")
	limitStr := c.DefaultQuery("limit", "20")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		respondError(c, http.StatusBadRequest, "limit 参数必须为正整数")
		return
	}
	if limit > 100 {
		limit = 100
	}

	products, err := h.exchangeService.SearchProductsContext(c.Request.Context(), keyword, limit)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, SearchProductsResponse{
		Products: dto.ToProductResponses(products),
		Total:    int64(len(products)),
	})
}

// GetCategoriesResponse 获取分类响应
type GetCategoriesResponse struct {
	Categories []string `json:"categories"`
}

// GetCategories 获取商品分类
func (h *ExchangeHandler) GetCategories(c *gin.Context) {
	categories, err := h.exchangeService.GetProductCategoriesContext(c.Request.Context())
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
	count, err := h.productService.UpdateProductsContext(c.Request.Context(), req.AccountID, userID, isAdmin)
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
