package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
)

// AnnouncementService 公告服务
type AnnouncementService struct {
	announcementRepo *repository.AnnouncementRepository
}

// NewAnnouncementService 创建公告服务
func NewAnnouncementService(announcementRepo *repository.AnnouncementRepository) *AnnouncementService {
	return &AnnouncementService{announcementRepo: announcementRepo}
}

// CreateAnnouncement 创建公告
func (s *AnnouncementService) CreateAnnouncement(title, content string, isPopup, isTop bool) (*models.Announcement, error) {
	return s.CreateAnnouncementContext(context.Background(), title, content, isPopup, isTop)
}

func (s *AnnouncementService) CreateAnnouncementContext(ctx context.Context, title, content string, isPopup, isTop bool) (*models.Announcement, error) {
	announcement := &models.Announcement{Title: title, Content: content, IsPopup: isPopup, IsTop: isTop, IsPublished: true}
	if err := s.announcementRepo.WithContext(ctx).Create(announcement); err != nil {
		return nil, err
	}
	return announcement, nil
}

// UpdateAnnouncement 更新公告
func (s *AnnouncementService) UpdateAnnouncement(id uint, title, content string, isPopup, isTop, isPublished bool) (*models.Announcement, error) {
	return s.UpdateAnnouncementContext(context.Background(), id, title, content, isPopup, isTop, isPublished)
}

func (s *AnnouncementService) UpdateAnnouncementContext(ctx context.Context, id uint, title, content string, isPopup, isTop, isPublished bool) (*models.Announcement, error) {
	repo := s.announcementRepo.WithContext(ctx)
	announcement, err := repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	announcement.Title = title
	announcement.Content = content
	announcement.IsPopup = isPopup
	announcement.IsTop = isTop
	announcement.IsPublished = isPublished
	if err := repo.Update(announcement); err != nil {
		return nil, err
	}
	return announcement, nil
}

// DeleteAnnouncement 删除公告
func (s *AnnouncementService) DeleteAnnouncement(id uint) error {
	return s.DeleteAnnouncementContext(context.Background(), id)
}

func (s *AnnouncementService) DeleteAnnouncementContext(ctx context.Context, id uint) error {
	return s.announcementRepo.WithContext(ctx).Delete(id)
}

// GetAnnouncement 获取公告详情
func (s *AnnouncementService) GetAnnouncement(id uint) (*models.Announcement, error) {
	return s.GetAnnouncementContext(context.Background(), id)
}

func (s *AnnouncementService) GetAnnouncementContext(ctx context.Context, id uint) (*models.Announcement, error) {
	return s.announcementRepo.WithContext(ctx).GetByID(id)
}

// GetAllAnnouncements 获取所有公告
func (s *AnnouncementService) GetAllAnnouncements() ([]*models.Announcement, error) {
	return s.GetAllAnnouncementsContext(context.Background())
}

func (s *AnnouncementService) GetAllAnnouncementsContext(ctx context.Context) ([]*models.Announcement, error) {
	return s.announcementRepo.WithContext(ctx).GetAll()
}

// GetPublishedAnnouncements 获取已发布的公告
func (s *AnnouncementService) GetPublishedAnnouncements() ([]*models.Announcement, error) {
	return s.GetPublishedAnnouncementsContext(context.Background())
}

func (s *AnnouncementService) GetPublishedAnnouncementsContext(ctx context.Context) ([]*models.Announcement, error) {
	return s.announcementRepo.WithContext(ctx).GetPublished()
}

// GetPopupAnnouncements 获取需要弹窗的公告
func (s *AnnouncementService) GetPopupAnnouncements() ([]*models.Announcement, error) {
	return s.GetPopupAnnouncementsContext(context.Background())
}

func (s *AnnouncementService) GetPopupAnnouncementsContext(ctx context.Context) ([]*models.Announcement, error) {
	return s.announcementRepo.WithContext(ctx).GetPopupAnnouncements()
}

// GetFirstPopupAnnouncement 获取第一个需要弹窗的公告
func (s *AnnouncementService) GetFirstPopupAnnouncement() (*models.Announcement, error) {
	return s.GetFirstPopupAnnouncementContext(context.Background())
}

func (s *AnnouncementService) GetFirstPopupAnnouncementContext(ctx context.Context) (*models.Announcement, error) {
	return s.announcementRepo.WithContext(ctx).GetFirstPopup()
}
