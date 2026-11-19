package handlers

import (
	"net/http"
	"strconv"

	"caiyun/internal/models"
	"caiyun/internal/services"
	"caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

type TaskDailySummaryResponse struct {
	Date     string                     `json:"date"`
	Accounts []*models.TaskDailySummary `json:"accounts"`
	Total    int64                      `json:"total"`
	Page     int                        `json:"page"`
	PageSize int                        `json:"page_size"`
}

// GetTaskDailySummary returns one row per account on the selected CST day.
// @Summary 获取每日任务汇总
// @Tags 任务管理
// @Param date query string false "日期 YYYY-MM-DD，默认今天"
// @Param page query int false "页码"
// @Param page_size query int false "每页账号数"
// @Success 200 {object} TaskDailySummaryResponse
// @Router /api/tasks/daily-summary [get]
func (h *TaskHandler) GetTaskDailySummary(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	start, _, err := services.TaskLogDayRange(c.Query("date"))
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page, limit = normalizePageLimit(page, limit, 20, 100)
	date := start.Format("2006-01-02")
	rows, total, err := h.taskService.GetTaskDailySummariesContext(c.Request.Context(), userID, date, page, limit)
	if err != nil {
		respondTaskHandlerError(c, err)
		return
	}
	response.Success(c, TaskDailySummaryResponse{Date: date, Accounts: rows, Total: total, Page: page, PageSize: limit})
}
