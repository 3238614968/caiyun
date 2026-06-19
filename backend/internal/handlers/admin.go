package handlers

import (
	"caiyun/internal/services"
	apiresponse "caiyun/pkg/response"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AdminHandler 管理员处理器
type AdminHandler struct {
	adminService *services.AdminService
}

// NewAdminHandler 创建管理员处理器
func NewAdminHandler(adminService *services.AdminService) *AdminHandler {
	return &AdminHandler{
		adminService: adminService,
	}
}

// GetAllUsers 获取所有用户
func (h *AdminHandler) GetAllUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	// 小于1时默认20，超过100时展示前20
	if size < 1 {
		size = 20
	} else if size > 100 {
		size = 20
	}

	users, total, err := h.adminService.GetAllUsers(page, size)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, gin.H{
		"users": users,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// GetAllAccounts 获取所有账号
func (h *AdminHandler) GetAllAccounts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	// 小于1时默认10，超过100时展示前20
	if pageSize < 1 {
		pageSize = 10
	} else if pageSize > 100 {
		pageSize = 20
	}

	accounts, total, err := h.adminService.GetAllAccounts(page, pageSize)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, gin.H{
		"accounts":  accounts,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// SearchAllAccounts 搜索所有账号（管理员用）
func (h *AdminHandler) SearchAllAccounts(c *gin.Context) {
	keyword := c.Query("keyword")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	// 小于1时默认20，超过100时展示前20
	if limit < 1 {
		limit = 20
	} else if limit > 100 {
		limit = 20
	}

	req := &services.SearchAllAccountsRequest{
		Keyword: keyword,
		Limit:   limit,
	}

	resp, err := h.adminService.SearchAllAccounts(req)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, resp)
}

// GetAccountSummaries 获取所有账号概况
func (h *AdminHandler) GetAccountSummaries(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	// 小于1时默认20，超过100时展示前20
	if pageSize < 1 {
		pageSize = 20
	} else if pageSize > 100 {
		pageSize = 20
	}

	summaries, total, err := h.adminService.GetAccountSummaries(page, pageSize)
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, gin.H{
		"summaries": summaries,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetAdminDashboard 获取管理员仪表盘数据
func (h *AdminHandler) GetAdminDashboard(c *gin.Context) {
	data, err := h.adminService.GetAdminDashboard()
	if err != nil {
		respondInternalServer(c)
		return
	}
	apiresponse.Success(c, data)
}

// UpdateUserRole 更新用户角色
func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的用户ID")
		return
	}

	var req services.UpdateUserRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.adminService.UpdateUserRole(uint(userID), &req); err != nil {
		if err == services.ErrUserNotFound {
			respondError(c, http.StatusNotFound, "用户不存在")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "角色更新成功")
}

// ResetUserPassword 管理员重置用户密码
func (h *AdminHandler) ResetUserPassword(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的用户ID")
		return
	}

	var req services.ResetUserPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.adminService.ResetUserPassword(uint(userID), &req); err != nil {
		switch {
		case errors.Is(err, services.ErrUserNotFound):
			respondError(c, http.StatusNotFound, "用户不存在")
		case errors.Is(err, services.ErrWeakPassword):
			respondError(c, http.StatusBadRequest, err.Error())
		default:
			respondInternalServer(c)
		}
		return
	}

	apiresponse.Message(c, "用户密码已重置")
}

// UpdateAccountStatusRequest 更新账号状态请求
type UpdateAccountStatusRequest = services.UpdateAccountStatusRequest

// UpdateAccountStatus 更新账号状态
func (h *AdminHandler) UpdateAccountStatus(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	var req services.UpdateAccountStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.adminService.UpdateAccountStatus(uint(accountID), &req); err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "账号状态更新成功")
}

// DeleteUser 删除用户
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的用户ID")
		return
	}

	currentUserID, exists := c.Get("user_id")
	if !exists {
		respondError(c, http.StatusUnauthorized, "未授权")
		return
	}

	if err := h.adminService.DeleteUser(uint(userID), currentUserID.(uint)); err != nil {
		if err == services.ErrCannotDeleteSelf {
			respondError(c, http.StatusBadRequest, "不能删除自己")
			return
		}
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "用户删除成功")
}

// DeleteAccount 删除账号
func (h *AdminHandler) DeleteAccount(c *gin.Context) {
	accountID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		respondError(c, http.StatusBadRequest, "无效的账号ID")
		return
	}

	if err := h.adminService.DeleteAccount(uint(accountID)); err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Message(c, "账号删除成功")
}

// GetStatsOverview 获取统计概览
func (h *AdminHandler) GetStatsOverview(c *gin.Context) {
	stats, err := h.adminService.GetStatsOverview()
	if err != nil {
		respondInternalServer(c)
		return
	}

	apiresponse.Success(c, gin.H{
		"user_count":    stats.UserCount,
		"account_count": stats.AccountCount,
		"total_cloud":   stats.TotalCloud,
		"active_tasks":  stats.ActiveTasks,
	})
}

// GetTaskConfigs 获取任务配置列表
func (h *AdminHandler) GetTaskConfigs(c *gin.Context) {
	configs, err := h.adminService.GetTaskConfigs()
	if err != nil {
		respondInternalServer(c)
		return
	}
	apiresponse.Success(c, gin.H{"configs": configs})
}

// UpdateTaskConfig 更新任务配置（上架/下架）
func (h *AdminHandler) UpdateTaskConfig(c *gin.Context) {
	taskType := c.Param("task_type")
	if taskType == "" {
		respondError(c, http.StatusBadRequest, "缺少任务类型")
		return
	}

	var req services.UpdateTaskConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.adminService.UpdateTaskConfig(taskType, &req); err != nil {
		respondInternalServer(c)
		return
	}

	action := "上架"
	if !req.IsEnabled {
		action = "下架"
	}
	apiresponse.Message(c, "任务已"+action)
}
