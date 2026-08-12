package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/repository"

	"github.com/google/uuid"
)

// RefreshToken preserves the legacy service API for non-HTTP callers that do
// not have a refresh credential. Production HTTP refresh uses RefreshWithToken.
func (s *AuthService) RefreshToken(userID uint) (*AuthResponse, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return s.issueAuthSession(context.Background(), user, SessionMetadata{})
}

// RefreshWithToken atomically rotates a refresh credential and issues an
// access JWT bound to the replacement sid. Presenting a consumed credential
// causes the repository to revoke all active sessions for that user.
func (s *AuthService) RefreshWithToken(ctx context.Context, rawRefreshToken string, metadata SessionMetadata) (*AuthResponse, error) {
	if s.sessionRepo == nil {
		return nil, ErrInvalidRefreshToken
	}
	normalized, err := normalizeRefreshCredential(rawRefreshToken)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	newRaw, newHash, err := newRefreshCredential()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	replacement := &models.RefreshSession{
		ID:               uuid.NewString(),
		RefreshTokenHash: newHash,
		DeviceInfo:       normalizeDeviceInfo(metadata.DeviceInfo),
		ExpiresAt:        now.Add(s.refreshExpiry),
	}
	current, err := s.sessionRepo.Rotate(ctx, hashRefreshCredential(normalized), replacement, now)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrRefreshSessionReused):
			return nil, ErrRefreshTokenReuse
		case errors.Is(err, repository.ErrRefreshSessionNotFound),
			errors.Is(err, repository.ErrRefreshSessionRevoked),
			errors.Is(err, repository.ErrRefreshSessionExpired),
			errors.Is(err, repository.ErrRefreshSessionVersionChanged):
			return nil, ErrInvalidRefreshToken
		default:
			return nil, fmt.Errorf("rotate refresh session: %w", err)
		}
	}

	user, err := s.userRepo.FindByID(current.UserID)
	if err != nil {
		return nil, fmt.Errorf("load refresh session user: %w", err)
	}
	if user.TokenVersion != replacement.TokenVersion {
		_ = s.sessionRepo.RevokeAll(ctx, user.ID, now)
		return nil, ErrInvalidRefreshToken
	}

	accessToken, err := s.jwtMgr.GenerateAccessToken(
		user.ID,
		user.Username,
		user.Role,
		user.TokenVersion,
		replacement.ID,
		s.jwtExpiry,
	)
	if err != nil {
		_ = s.sessionRepo.Revoke(ctx, replacement.ID, user.ID, now)
		return nil, err
	}
	user.Password = ""
	return &AuthResponse{
		Token:            accessToken,
		ExpiresAt:        now.Add(s.jwtExpiry).Unix(),
		RefreshToken:     newRaw,
		RefreshExpiresAt: replacement.ExpiresAt.Unix(),
		User:             user,
	}, nil
}

// RevokeSession immediately invalidates both the refresh credential and every
// access JWT carrying this sid (middleware checks session activity).
func (s *AuthService) RevokeSession(ctx context.Context, userID uint, sessionID string) error {
	if s.sessionRepo == nil || userID == 0 || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return s.sessionRepo.Revoke(ctx, strings.TrimSpace(sessionID), userID, time.Now())
}

func (s *AuthService) RevokeAllSessions(ctx context.Context, userID uint) error {
	if s.sessionRepo == nil || userID == 0 {
		return nil
	}
	return s.sessionRepo.RevokeAll(ctx, userID, time.Now())
}

func (s *AuthService) issueAuthSession(ctx context.Context, user *models.User, metadata SessionMetadata) (*AuthResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	if s.sessionRepo == nil {
		token, err := s.jwtMgr.GenerateToken(user.ID, user.Username, user.Role, user.TokenVersion, s.jwtExpiry)
		if err != nil {
			return nil, err
		}
		user.Password = ""
		return &AuthResponse{Token: token, ExpiresAt: now.Add(s.jwtExpiry).Unix(), User: user}, nil
	}

	rawRefreshToken, refreshHash, err := newRefreshCredential()
	if err != nil {
		return nil, err
	}
	session := &models.RefreshSession{
		ID:               uuid.NewString(),
		UserID:           user.ID,
		RefreshTokenHash: refreshHash,
		TokenVersion:     user.TokenVersion,
		DeviceInfo:       normalizeDeviceInfo(metadata.DeviceInfo),
		ExpiresAt:        now.Add(s.refreshExpiry),
	}
	accessToken, err := s.jwtMgr.GenerateAccessToken(
		user.ID,
		user.Username,
		user.Role,
		user.TokenVersion,
		session.ID,
		s.jwtExpiry,
	)
	if err != nil {
		return nil, err
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create refresh session: %w", err)
	}

	user.Password = ""
	return &AuthResponse{
		Token:            accessToken,
		ExpiresAt:        now.Add(s.jwtExpiry).Unix(),
		RefreshToken:     rawRefreshToken,
		RefreshExpiresAt: session.ExpiresAt.Unix(),
		User:             user,
	}, nil
}

func newRefreshCredential() (raw string, hash string, err error) {
	var tokenBytes [32]byte
	if _, err = rand.Read(tokenBytes[:]); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(tokenBytes[:])
	return raw, hashRefreshCredential(raw), nil
}

func normalizeRefreshCredential(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return "", ErrInvalidRefreshToken
	}
	return raw, nil
}

func hashRefreshCredential(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeDeviceInfo(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 255 {
		runes = runes[:255]
	}
	return string(runes)
}
