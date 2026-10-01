package repository

import (
	"caiyun/internal/models"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var ErrDuplicateAccountIdentity = errors.New("account identity already exists")

// FindLoginMetadata includes a removed account but never decrypts old
// credentials. A new login can replace unreadable credentials safely.
func (r *AccountRepository) FindLoginMetadata(phone string, userID uint) (*models.Account, error) {
	var account models.Account
	result := r.db.Unscoped().Select(accountListColumns).
		Where("phone = ? AND user_id = ?", phone, userID).
		Order("deleted_at IS NULL DESC, id ASC").Limit(1).Find(&account)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &account, nil
}

// SaveLogin restores the same account ID and its history, replacing only login
// fields. A unique-key race is retried in a fresh service transaction.
func (r *AccountRepository) SaveLogin(account *models.Account) error {
	existing, err := r.FindLoginMetadata(account.Phone, account.UserID)
	if err != nil {
		return err
	}
	if existing == nil {
		if err := r.Create(account); err != nil {
			if isDatabaseDuplicate(err) {
				return fmt.Errorf("%w: %v", ErrDuplicateAccountIdentity, err)
			}
			return err
		}
		return nil
	}
	authValue, err := encryptCredentialValue(account.Auth)
	if err != nil {
		return err
	}
	token, err := encryptCredentialValue(account.Token)
	if err != nil {
		return err
	}
	jwtToken, err := encryptCredentialValue(account.JWTToken)
	if err != nil {
		return err
	}
	if account.Remark == "" {
		account.Remark = existing.Remark
	}
	now := time.Now()
	result := r.db.Unscoped().Model(&models.Account{}).
		Where("id = ? AND user_id = ? AND phone = ?", existing.ID, account.UserID, account.Phone).
		Updates(map[string]interface{}{
			"auth": authValue, "token": token, "jwt_token": jwtToken,
			"platform": account.Platform, "expire_at": account.ExpireAt, "remark": account.Remark,
			"deleted_at": nil, "is_active": true, "jwt_error_count": 0, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	account.ID, account.CloudCount, account.CreatedAt = existing.ID, existing.CloudCount, existing.CreatedAt
	account.UpdatedAt, account.DeletedAt = now, gorm.DeletedAt{}
	account.IsActive, account.JWTErrorCount = true, 0
	return nil
}

// ReplaceLoginByAccountID also clears an old JWT when a new login has not
// fetched one yet. Updating credentials must not require decrypting old data.
func (r *ExchangeAccountRepository) ReplaceLoginByAccountID(accountID uint, authValue, token, jwtToken string) error {
	encryptedAuth, err := encryptCredentialValue(authValue)
	if err != nil {
		return err
	}
	encryptedToken, err := encryptCredentialValue(token)
	if err != nil {
		return err
	}
	encryptedJWT, err := encryptCredentialValue(jwtToken)
	if err != nil {
		return err
	}
	return r.db.Model(&models.ExchangeAccount{}).Where("account_id = ?", accountID).
		Updates(map[string]interface{}{"auth": encryptedAuth, "token": encryptedToken, "jwt_token": encryptedJWT}).Error
}
