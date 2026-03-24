package models

import (
	"time"
)

// Announcement 公告模型
type Announcement struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Title       string    `gorm:"size:200;not null" json:"title"`
	Content     string    `gorm:"type:text;not null" json:"content"`
	IsPopup     bool      `gorm:"default:false" json:"is_popup"`      // 是否弹窗显示
	IsTop       bool      `gorm:"default:false" json:"is_top"`        // 是否置顶
	IsPublished bool      `gorm:"default:true" json:"is_published"`   // 是否发布
	PopupCount  int       `gorm:"default:0" json:"popup_count"`       // 弹窗显示次数
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (Announcement) TableName() string {
	return "announcements"
}
