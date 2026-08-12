// Package dto owns transport-safe API representations. Persistence models stay
// inside the repository and service layers and are mapped explicitly here.
package dto

import (
	"caiyun/internal/models"
	"time"
)

// UserSummary is the public account owner view. It intentionally excludes
// password, token-version, deleted state and nested account collections.
type UserSummary struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// AccountResponse is the stable public account representation. Credential
// material is intentionally absent even if a persistence model later gains
// additional fields.
type AccountResponse struct {
	ID            uint         `json:"id"`
	UserID        uint         `json:"user_id"`
	Phone         string       `json:"phone"`
	Platform      string       `json:"platform"`
	ExpireAt      int64        `json:"expire_at"`
	CloudCount    int          `json:"cloud_count"`
	Remark        string       `json:"remark"`
	IsActive      bool         `json:"is_active"`
	JWTErrorCount int          `json:"jwt_error_count"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	User          *UserSummary `json:"user,omitempty"`
}

// ToAccountResponse maps one persistence account to its public API shape.
func ToAccountResponse(account *models.Account) *AccountResponse {
	if account == nil {
		return nil
	}
	response := &AccountResponse{
		ID:            account.ID,
		UserID:        account.UserID,
		Phone:         account.Phone,
		Platform:      account.Platform,
		ExpireAt:      account.ExpireAt,
		CloudCount:    account.CloudCount,
		Remark:        account.Remark,
		IsActive:      account.IsActive,
		JWTErrorCount: account.JWTErrorCount,
		CreatedAt:     account.CreatedAt,
		UpdatedAt:     account.UpdatedAt,
	}
	if account.User.ID != 0 {
		response.User = &UserSummary{
			ID:       account.User.ID,
			Username: account.User.Username,
			Email:    account.User.Email,
			Role:     account.User.Role,
		}
	}
	return response
}

// ToAccountResponses maps a list without ever exposing persistence instances.
func ToAccountResponses(accounts []*models.Account) []*AccountResponse {
	responses := make([]*AccountResponse, len(accounts))
	for index, account := range accounts {
		responses[index] = ToAccountResponse(account)
	}
	return responses
}
