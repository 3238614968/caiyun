package models

import "time"

// RefreshSession stores one rotating refresh credential. The raw refresh
// token is never persisted; RefreshTokenHash contains its SHA-256 digest.
//
// Each successful refresh creates a replacement session and revokes the
// previous one. Keeping the replacement link lets the repository detect
// refresh-token reuse and revoke the whole session family.
type RefreshSession struct {
	ID                  string     `gorm:"column:id;primaryKey;size:36" json:"id"`
	UserID              uint       `gorm:"column:user_id;not null;index" json:"user_id"`
	RefreshTokenHash    string     `gorm:"column:refresh_token_hash;size:64;not null;uniqueIndex" json:"-"`
	TokenVersion        int        `gorm:"column:token_version;not null" json:"-"`
	DeviceInfo          string     `gorm:"column:device_info;size:255" json:"device_info,omitempty"`
	ExpiresAt           time.Time  `gorm:"column:expires_at;not null;index" json:"expires_at"`
	RevokedAt           *time.Time `gorm:"column:revoked_at;index" json:"revoked_at,omitempty"`
	ReplacedBySessionID *string    `gorm:"column:replaced_by_session_id;size:36;index" json:"replaced_by_session_id,omitempty"`
	LastUsedAt          *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	CreatedAt           time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (RefreshSession) TableName() string {
	return "refresh_sessions"
}
