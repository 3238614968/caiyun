package repository

import (
	"caiyun/internal/models"
	"context"
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

// Create 创建兑换账号
func (r *ExchangeAccountRepository) Create(account *models.ExchangeAccount) error {
	return r.db.Create(account).Error
}

// Update 更新兑换账号
func (r *ExchangeAccountRepository) Update(account *models.ExchangeAccount) error {
	return r.db.Save(account).Error
}

// Delete 删除兑换账号
func (r *ExchangeAccountRepository) Delete(id uint) error {
	return r.db.Delete(&models.ExchangeAccount{}, id).Error
}

// GetByID 根据 ID 获取兑换账号
func (r *ExchangeAccountRepository) GetByID(id uint) (*models.ExchangeAccount, error) {
	var account models.ExchangeAccount
	err := r.db.Preload("Tasks").Preload("Tasks.Product").First(&account, id).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// GetByUserID 根据用户 ID 获取所有兑换账号
func (r *ExchangeAccountRepository) GetByUserID(userID uint) ([]*models.ExchangeAccount, error) {
	var accounts []*models.ExchangeAccount
	err := r.db.Where("user_id = ?", userID).
		Preload("Tasks").Preload("Tasks.Product").
		Order("created_at DESC").
		Find(&accounts).Error
	return accounts, err
}

// GetActiveByUserID 根据用户 ID 获取活跃的兑换账号
func (r *ExchangeAccountRepository) GetActiveByUserID(userID uint) ([]*models.ExchangeAccount, error) {
	var accounts []*models.ExchangeAccount
	err := r.db.Where("user_id = ? AND is_active = ?", userID, true).
		Preload("Tasks").Preload("Tasks.Product").
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

// UpdateLastExchangeAt 更新最后抢兑时间
func (r *ExchangeAccountRepository) UpdateLastExchangeAt(id uint, t time.Time) error {
	return r.db.Model(&models.ExchangeAccount{}).
		Where("id = ?", id).
		Update("last_exchange_at", t).Error
}

// UpdateAuthByAccountID 同步云盘主账号刷新后的鉴权信息到对应抢兑账号。
func (r *ExchangeAccountRepository) UpdateAuthByAccountID(accountID uint, auth, token, jwtToken string) error {
	updates := map[string]interface{}{
		"auth":  auth,
		"token": token,
	}
	if jwtToken != "" {
		updates["jwt_token"] = jwtToken
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
		Preload("Tasks").Preload("Tasks.Product").
		Order("created_at DESC").
		Find(&accounts).Error
	return accounts, err
}

// GetAll 获取所有兑换账号（管理员用）
func (r *ExchangeAccountRepository) GetAll() ([]*models.ExchangeAccount, error) {
	var accounts []*models.ExchangeAccount
	err := r.db.Preload("Tasks").Preload("Tasks.Product").Order("created_at DESC").Find(&accounts).Error
	return accounts, err
}
