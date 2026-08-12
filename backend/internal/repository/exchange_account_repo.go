package repository

import (
	"caiyun/internal/models"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ExchangeAccountRepository 兑换账号数据访问层
type ExchangeAccountRepository struct {
	db *gorm.DB
}

func NewExchangeAccountRepository(db *gorm.DB) *ExchangeAccountRepository {
	return &ExchangeAccountRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *ExchangeAccountRepository) WithContext(ctx context.Context) *ExchangeAccountRepository {
	if ctx == nil {
		return r
	}
	return &ExchangeAccountRepository{db: r.db.WithContext(ctx)}
}

// Create 创建兑换账号。
// 这里额外在仓储层做一次凭证加密防御，避免未来改成 Updates(map) / SkipHooks 时把明文直接写入数据库。
func (r *ExchangeAccountRepository) Create(account *models.ExchangeAccount) error {
	if account == nil {
		return fmt.Errorf("兑换账号为空")
	}
	persisted, err := cloneExchangeAccountForWrite(account)
	if err != nil {
		return err
	}
	if err := mapExchangeRuleWriteError(r.db.Create(persisted).Error); err != nil {
		return err
	}
	account.ID = persisted.ID
	account.CreatedAt = persisted.CreatedAt
	account.UpdatedAt = persisted.UpdatedAt
	account.LastExchangeAt = persisted.LastExchangeAt
	return nil
}

// Update 更新兑换账号。
// 使用精确字段 Updates，避免 Save 全量覆盖并发修改的其他列。
func (r *ExchangeAccountRepository) Update(account *models.ExchangeAccount) error {
	if account == nil {
		return fmt.Errorf("兑换账号为空")
	}
	updates, err := buildExchangeAccountUpdates(account)
	if err != nil {
		return err
	}
	return mapExchangeRuleWriteError(r.db.Model(&models.ExchangeAccount{}).
		Where("id = ?", account.ID).
		Updates(updates).Error)
}

// Delete 删除兑换账号
func (r *ExchangeAccountRepository) Delete(id uint) error {
	return r.db.Delete(&models.ExchangeAccount{}, id).Error
}

// GetByID 根据 ID 获取兑换账号
func (r *ExchangeAccountRepository) GetByID(id uint) (*models.ExchangeAccount, error) {
	var account models.ExchangeAccount
	err := r.db.Preload("Tasks", func(db *gorm.DB) *gorm.DB {
		return db.Preload("Product")
	}).First(&account, id).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// GetByUserID 根据用户 ID 获取所有兑换账号
func (r *ExchangeAccountRepository) GetByUserID(userID uint) ([]*models.ExchangeAccount, error) {
	var accounts []*models.ExchangeAccount
	err := r.db.Where("user_id = ?", userID).
		Preload("Tasks").
		Order("created_at DESC").
		Find(&accounts).Error
	return accounts, err
}

// GetActiveByUserID 根据用户 ID 获取活跃的兑换账号
func (r *ExchangeAccountRepository) GetActiveByUserID(userID uint) ([]*models.ExchangeAccount, error) {
	var accounts []*models.ExchangeAccount
	err := r.db.Where("user_id = ? AND is_active = ?", userID, true).
		Preload("Tasks").
		Order("created_at DESC").
		Find(&accounts).Error
	return accounts, err
}

// GetByAccountID 根据云盘账号 ID 获取兑换账号
func (r *ExchangeAccountRepository) GetByAccountID(accountID uint) (*models.ExchangeAccount, error) {
	var account models.ExchangeAccount
	err := r.db.Where("account_id = ?", accountID).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// FindByAccountID 根据云盘账号 ID 获取兑换账号；不存在时返回 nil，便于自动创建/复用。
func (r *ExchangeAccountRepository) FindByAccountID(accountID uint) (*models.ExchangeAccount, error) {
	var account models.ExchangeAccount
	result := r.db.Where("account_id = ?", accountID).Limit(1).Find(&account)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &account, nil
}

// UpdateLastExchangeAt 更新最后抢兑时间
func (r *ExchangeAccountRepository) UpdateLastExchangeAt(id uint, t time.Time) error {
	return r.db.Model(&models.ExchangeAccount{}).
		Where("id = ?", id).
		Update("last_exchange_at", t).Error
}

// UpdateAuthByAccountID 同步云盘主账号刷新后的鉴权信息到对应抢兑账号。
func (r *ExchangeAccountRepository) UpdateAuthByAccountID(accountID uint, auth, token, jwtToken string) error {
	encryptedAuth, err := encryptCredentialValue(auth)
	if err != nil {
		return err
	}
	encryptedToken, err := encryptCredentialValue(token)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"auth":  encryptedAuth,
		"token": encryptedToken,
	}
	if jwtToken != "" {
		encryptedJWTToken, err := encryptCredentialValue(jwtToken)
		if err != nil {
			return err
		}
		updates["jwt_token"] = encryptedJWTToken
	}
	return r.db.Model(&models.ExchangeAccount{}).
		Where("account_id = ?", accountID).
		Updates(updates).Error
}

// Count 获取用户的兑换账号数量
func (r *ExchangeAccountRepository) Count(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.ExchangeAccount{}).
		Where("user_id = ? AND is_active = ?", userID, true).
		Count(&count).Error
	return count, err
}

// ExistsByAccountID 检查云盘账号是否已添加为兑换账号
func (r *ExchangeAccountRepository) ExistsByAccountID(accountID uint) bool {
	var count int64
	r.db.Model(&models.ExchangeAccount{}).
		Where("account_id = ?", accountID).
		Count(&count)
	return count > 0
}

// GetAllActive 获取所有活跃的兑换账号
func (r *ExchangeAccountRepository) GetAllActive() ([]*models.ExchangeAccount, error) {
	var accounts []*models.ExchangeAccount
	err := r.db.Where("is_active = ?", true).
		Preload("Tasks").
		Order("created_at DESC").
		Find(&accounts).Error
	return accounts, err
}

// GetAll 获取所有兑换账号（管理员用）
func (r *ExchangeAccountRepository) GetAll() ([]*models.ExchangeAccount, error) {
	var accounts []*models.ExchangeAccount
	err := r.db.Preload("Tasks").Order("created_at DESC").Find(&accounts).Error
	return accounts, err
}

func cloneExchangeAccountForWrite(account *models.ExchangeAccount) (*models.ExchangeAccount, error) {
	clone := *account
	encryptedAuth, err := encryptCredentialValue(clone.Auth)
	if err != nil {
		return nil, err
	}
	encryptedToken, err := encryptCredentialValue(clone.Token)
	if err != nil {
		return nil, err
	}
	encryptedJWTToken, err := encryptCredentialValue(clone.JWTToken)
	if err != nil {
		return nil, err
	}
	clone.Auth = encryptedAuth
	clone.Token = encryptedToken
	clone.JWTToken = encryptedJWTToken
	return &clone, nil
}

func buildExchangeAccountUpdates(account *models.ExchangeAccount) (map[string]interface{}, error) {
	encryptedAuth, err := encryptCredentialValue(account.Auth)
	if err != nil {
		return nil, err
	}
	encryptedToken, err := encryptCredentialValue(account.Token)
	if err != nil {
		return nil, err
	}
	encryptedJWTToken, err := encryptCredentialValue(account.JWTToken)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"phone":            account.Phone,
		"auth":             encryptedAuth,
		"token":            encryptedToken,
		"jwt_token":        encryptedJWTToken,
		"remark":           account.Remark,
		"exchange_time_1":  account.ExchangeTime1,
		"exchange_time_2":  account.ExchangeTime2,
		"is_active":        account.IsActive,
		"last_exchange_at": account.LastExchangeAt,
	}, nil
}
