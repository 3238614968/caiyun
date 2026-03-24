package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
)

// AnnouncementService 公告服务
type AnnouncementService struct {
	announcementRepo *repository.AnnouncementRepository
}

// NewAnnouncementService 创建公告服务
func NewAnnouncementService(announcementRepo *repository.AnnouncementRepository) *AnnouncementService {
	return &AnnouncementService{
		announcementRepo: announcementRepo,
	}
}

// CreateAnnouncement 创建公告
func (s *AnnouncementService) CreateAnnouncement(title, content string, isPopup, isTop bool) (*models.Announcement, error) {
	announcement := &models.Announcement{
		Title:       title,
		Content:     content,
		IsPopup:     isPopup,
		IsTop:       isTop,
		IsPublished: true,
	}

	if err := s.announcementRepo.Create(announcement); err != nil {
		return nil, err
	}

	return announcement, nil
}

// UpdateAnnouncement 更新公告
func (s *AnnouncementService) UpdateAnnouncement(id uint, title, content string, isPopup, isTop, isPublished bool) (*models.Announcement, error) {
	announcement, err := s.announcementRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	announcement.Title = title
	announcement.Content = content
	announcement.IsPopup = isPopup
	announcement.IsTop = isTop
	announcement.IsPublished = isPublished

	if err := s.announcementRepo.Update(announcement); err != nil {
		return nil, err
	}

	return announcement, nil
}

// DeleteAnnouncement 删除公告
func (s *AnnouncementService) DeleteAnnouncement(id uint) error {
	return s.announcementRepo.Delete(id)
}

// GetAnnouncement 获取公告详情
func (s *AnnouncementService) GetAnnouncement(id uint) (*models.Announcement, error) {
	return s.announcementRepo.GetByID(id)
}

// GetAllAnnouncements 获取所有公告
func (s *AnnouncementService) GetAllAnnouncements() ([]*models.Announcement, error) {
	return s.announcementRepo.GetAll()
}

// GetPublishedAnnouncements 获取已发布的公告
func (s *AnnouncementService) GetPublishedAnnouncements() ([]*models.Announcement, error) {
	return s.announcementRepo.GetPublished()
}

// GetPopupAnnouncements 获取需要弹窗的公告
func (s *AnnouncementService) GetPopupAnnouncements() ([]*models.Announcement, error) {
	return s.announcementRepo.GetPopupAnnouncements()
}

// GetFirstPopupAnnouncement 获取第一个需要弹窗的公告
func (s *AnnouncementService) GetFirstPopupAnnouncement() (*models.Announcement, error) {
	return s.announcementRepo.GetFirstPopup()
}
