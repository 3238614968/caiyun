package repository

import (
	"context"
	"errors"
	"time"

	"caiyun/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrRefreshSessionNotFound       = errors.New("refresh session not found")
	ErrRefreshSessionRevoked        = errors.New("refresh session revoked")
	ErrRefreshSessionExpired        = errors.New("refresh session expired")
	ErrRefreshSessionReused         = errors.New("refresh session reused")
	ErrRefreshSessionVersionChanged = errors.New("refresh session token version changed")
)

// RefreshSessionRepository persists rotating refresh tokens. All methods use
// the request context so database waits are cancelled when the caller exits.
type RefreshSessionRepository struct {
	db *gorm.DB
}

func NewRefreshSessionRepository(db *gorm.DB) *RefreshSessionRepository {
	return &RefreshSessionRepository{db: db}
}

func (r *RefreshSessionRepository) Create(ctx context.Context, session *models.RefreshSession) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.db.WithContext(ctx).Create(session).Error
}

// Rotate atomically consumes oldTokenHash and creates replacement. Concurrent
// use of an already consumed token is treated as credential reuse and revokes
// every active refresh session for that user. Security revocations are
// committed before the sentinel error is returned.
func (r *RefreshSessionRepository) Rotate(
	ctx context.Context,
	oldTokenHash string,
	replacement *models.RefreshSession,
	now time.Time,
) (*models.RefreshSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var current models.RefreshSession
	var outcome error
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		findErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("refresh_token_hash = ?", oldTokenHash).
			First(&current).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			outcome = ErrRefreshSessionNotFound
			return nil
		}
		if findErr != nil {
			return findErr
		}

		if current.RevokedAt != nil {
			// A replaced token being presented again is strong evidence that a
			// copied refresh credential is in use. Revoke all descendants too.
			if current.ReplacedBySessionID != nil {
				if err := revokeAllRefreshSessions(tx, current.UserID, now); err != nil {
					return err
				}
				outcome = ErrRefreshSessionReused
				return nil
			}
			outcome = ErrRefreshSessionRevoked
			return nil
		}
		if !now.Before(current.ExpiresAt) {
			if err := tx.Model(&models.RefreshSession{}).
				Where("id = ? AND revoked_at IS NULL", current.ID).
				Update("revoked_at", now).Error; err != nil {
				return err
			}
			outcome = ErrRefreshSessionExpired
			return nil
		}

		var user models.User
		if err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			Select("id", "token_version").
			First(&user, current.UserID).Error; err != nil {
			return err
		}
		if user.TokenVersion != current.TokenVersion {
			if err := revokeAllRefreshSessions(tx, current.UserID, now); err != nil {
				return err
			}
			outcome = ErrRefreshSessionVersionChanged
			return nil
		}

		replacement.UserID = current.UserID
		replacement.TokenVersion = current.TokenVersion
		if err := tx.Create(replacement).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.RefreshSession{}).
			Where("id = ? AND revoked_at IS NULL", current.ID).
			Updates(map[string]interface{}{
				"revoked_at":             now,
				"last_used_at":           now,
				"replaced_by_session_id": replacement.ID,
			}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if outcome != nil {
		return nil, outcome
	}
	return &current, nil
}

func (r *RefreshSessionRepository) IsActive(ctx context.Context, sessionID string, userID uint, now time.Time) (bool, error) {
	if sessionID == "" || userID == 0 {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var count int64
	err := r.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, userID, now).
		Count(&count).Error
	return count == 1, err
}

func (r *RefreshSessionRepository) Revoke(ctx context.Context, sessionID string, userID uint, now time.Time) error {
	if sessionID == "" || userID == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return r.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).
		Update("revoked_at", now).Error
}

func (r *RefreshSessionRepository) RevokeAll(ctx context.Context, userID uint, now time.Time) error {
	if userID == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return revokeAllRefreshSessions(r.db.WithContext(ctx), userID, now)
}

func revokeAllRefreshSessions(db *gorm.DB, userID uint, now time.Time) error {
	return db.Model(&models.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", now).Error
}
