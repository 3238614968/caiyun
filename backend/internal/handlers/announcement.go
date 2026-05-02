package handlers

import (
	"caiyun/internal/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// AnnouncementHandler 公告处理器
type AnnouncementHandler struct {
	announcementService *services.AnnouncementService
}

// NewAnnouncementHandler 创建公告处理器
func NewAnnouncementHandler(announcementService *services.AnnouncementService) *AnnouncementHandler {
	return &AnnouncementHandler{
		announcementService: announcementService,
	}
}

// CreateAnnouncementRequest 创建公告请求
type CreateAnnouncementRequest struct {
	Title   string `json:"title" binding:"required"`
	Content string `json:"content" binding:"required"`
	IsPopup bool   `json:"is_popup"`
	IsTop   bool   `json:"is_top"`
}

// CreateAnnouncement 创建公告
func (h *AnnouncementHandler) CreateAnnouncement(c *gin.Context) {
	var req CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "请求参数错误: " + err.Error()})
		return
	}

	announcement, err := h.announcementService.CreateAnnouncement(req.Title, req.Content, req.IsPopup, req.IsTop)
	if err != nil {
		c.JSON(http.StatusInternalServerError, InternalServerErrorResponse())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "创建成功",
		"announcement": announcement,
	})
}

// UpdateAnnouncementRequest 更新公告请求
type UpdateAnnouncementRequest struct {
	Title       string `json:"title" binding:"required"`
	Content     string `json:"content" binding:"required"`
	IsPopup     bool   `json:"is_popup"`
	IsTop       bool   `json:"is_top"`
	IsPublished bool   `json:"is_published"`
}

// UpdateAnnouncement 更新公告
func (h *AnnouncementHandler) UpdateAnnouncement(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "无效的ID"})
		return
	}

	var req UpdateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "请求参数错误: " + err.Error()})
		return
	}

	announcement, err := h.announcementService.UpdateAnnouncement(uint(id), req.Title, req.Content, req.IsPopup, req.IsTop, req.IsPublished)
	if err != nil {
		c.JSON(http.StatusInternalServerError, InternalServerErrorResponse())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "更新成功",
		"announcement": announcement,
	})
}

// DeleteAnnouncement 删除公告
func (h *AnnouncementHandler) DeleteAnnouncement(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "无效的ID"})
		return
	}

	if err := h.announcementService.DeleteAnnouncement(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, InternalServerErrorResponse())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "删除成功",
	})
}

// GetAnnouncement 获取公告详情
func (h *AnnouncementHandler) GetAnnouncement(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Message: "无效的ID"})
		return
	}

	announcement, err := h.announcementService.GetAnnouncement(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Message: "公告不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"announcement": announcement,
	})
}

// GetAllAnnouncements 获取所有公告
func (h *AnnouncementHandler) GetAllAnnouncements(c *gin.Context) {
	announcements, err := h.announcementService.GetAllAnnouncements()
	if err != nil {
		c.JSON(http.StatusInternalServerError, InternalServerErrorResponse())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"announcements": announcements,
		"total":         len(announcements),
	})
}

// GetPublishedAnnouncements 获取已发布的公告
func (h *AnnouncementHandler) GetPublishedAnnouncements(c *gin.Context) {
	announcements, err := h.announcementService.GetPublishedAnnouncements()
	if err != nil {
		c.JSON(http.StatusInternalServerError, InternalServerErrorResponse())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"announcements": announcements,
		"total":         len(announcements),
	})
}

// GetPopupAnnouncement 获取弹窗公告列表
func (h *AnnouncementHandler) GetPopupAnnouncement(c *gin.Context) {
	announcements, err := h.announcementService.GetPopupAnnouncements()
	if err != nil || len(announcements) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"has_popup":     false,
			"announcements": []interface{}{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"has_popup":     true,
		"announcements": announcements,
	})
}
