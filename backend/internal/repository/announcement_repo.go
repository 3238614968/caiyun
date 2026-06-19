package repository

import (
	"caiyun/internal/models"
	"context"

	"gorm.io/gorm"
)

// AnnouncementRepository 公告数据访问层
type AnnouncementRepository struct {
	db *gorm.DB
}

// NewAnnouncementRepository 创建公告仓库
func NewAnnouncementRepository(db *gorm.DB) *AnnouncementRepository {
	return &AnnouncementRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *AnnouncementRepository) WithContext(ctx context.Context) *AnnouncementRepository {
	if ctx == nil {
		return r
	}
	return &AnnouncementRepository{db: r.db.WithContext(ctx)}
}

// Create 创建公告
func (r *AnnouncementRepository) Create(announcement *models.Announcement) error {
	return r.db.Create(announcement).Error
}

// Update 更新公告
func (r *AnnouncementRepository) Update(announcement *models.Announcement) error {
	return r.db.Save(announcement).Error
}

// Delete 删除公告
func (r *AnnouncementRepository) Delete(id uint) error {
	return r.db.Delete(&models.Announcement{}, id).Error
}

// GetByID 根据ID获取公告
func (r *AnnouncementRepository) GetByID(id uint) (*models.Announcement, error) {
	var announcement models.Announcement
	err := r.db.First(&announcement, id).Error
	if err != nil {
		return nil, err
	}
	return &announcement, nil
}

// GetAll 获取所有公告
func (r *AnnouncementRepository) GetAll() ([]*models.Announcement, error) {
	var announcements []*models.Announcement
	err := r.db.Order("is_top DESC, created_at DESC").Find(&announcements).Error
	return announcements, err
}

// GetPublished 获取已发布的公告
func (r *AnnouncementRepository) GetPublished() ([]*models.Announcement, error) {
	var announcements []*models.Announcement
	err := r.db.Where("is_published = ?", true).
		Order("is_top DESC, created_at DESC").
		Find(&announcements).Error
	return announcements, err
}

// GetPopupAnnouncements 获取需要弹窗显示的公告
func (r *AnnouncementRepository) GetPopupAnnouncements() ([]*models.Announcement, error) {
	var announcements []*models.Announcement
	err := r.db.Where("is_published = ? AND is_popup = ?", true, true).
		Order("is_top DESC, created_at DESC").
		Find(&announcements).Error
	return announcements, err
}

// GetFirstPopup 获取第一个需要弹窗的公告（置顶优先）
func (r *AnnouncementRepository) GetFirstPopup() (*models.Announcement, error) {
	var announcement models.Announcement
	err := r.db.Where("is_published = ? AND is_popup = ?", true, true).
		Order("is_top DESC, created_at DESC").
		First(&announcement).Error
	if err != nil {
		return nil, err
	}
	return &announcement, nil
}
