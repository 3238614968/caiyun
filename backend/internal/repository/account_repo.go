package repository

import (
	"caiyun/internal/models"
	"context"

	"gorm.io/gorm"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *AccountRepository) WithContext(ctx context.Context) *AccountRepository {
	if ctx == nil {
		return r
	}
	return &AccountRepository{db: r.db.WithContext(ctx)}
}

// Create 创建账号
func (r *AccountRepository) Create(account *models.Account) error {
	return r.db.Create(account).Error
}

// FindByID 根据 ID 查找账号
func (r *AccountRepository) FindByID(id uint) (*models.Account, error) {
	var account models.Account
	err := r.db.Preload("User").First(&account, id).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// GetByID 根据 ID 查找账号（FindByID 的别名）
func (r *AccountRepository) GetByID(id uint) (*models.Account, error) {
	return r.FindByID(id)
}

// GetAll 获取所有账号
func (r *AccountRepository) GetAll() ([]*models.Account, error) {
	var accounts []*models.Account
	err := r.db.Find(&accounts).Error
	return accounts, err
}

// GetAllActive 获取所有活跃账号
func (r *AccountRepository) GetAllActive() ([]*models.Account, error) {
	var accounts []*models.Account
	err := r.db.Where("is_active = ?", true).Find(&accounts).Error
	return accounts, err
}

// SearchAll 搜索所有账号（管理员用）
func (r *AccountRepository) SearchAll(keyword string, limit int) ([]*models.Account, error) {
	var accounts []*models.Account
	query := r.db.Model(&models.Account{})

	if keyword != "" {
		query = query.Where("phone LIKE ? OR remark LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}

	err := query.Preload("User").Limit(limit).Find(&accounts).Error
	return accounts, err
}

// CountActive 统计当前未删除且启用的账号数量。
func (r *AccountRepository) CountActive() (int64, error) {
	var total int64
	err := r.db.Model(&models.Account{}).
		Where("is_active = ?", true).
		Count(&total).Error
	return total, err
}

// SumCloudCount 统计所有未删除账号当前云朵总数。
func (r *AccountRepository) SumCloudCount() (int, error) {
	var total int
	err := r.db.Model(&models.Account{}).
		Select("COALESCE(SUM(cloud_count), 0)").
		Scan(&total).Error
	return total, err
}

// TopByCloudCount 按云朵数量倒序返回账号榜单，预加载归属用户以避免 N+1 查询。
func (r *AccountRepository) TopByCloudCount(limit int) ([]*models.Account, error) {
	var accounts []*models.Account
	if limit <= 0 {
		limit = 20
	}
	err := r.db.Model(&models.Account{}).
		Preload("User").
		Order("cloud_count DESC").
		Limit(limit).
		Find(&accounts).Error
	return accounts, err
}

// FindByUserID 根据用户ID查找所有账号
func (r *AccountRepository) FindByUserID(userID uint) ([]*models.Account, error) {
	var accounts []*models.Account
	err := r.db.Where("user_id = ?", userID).Find(&accounts).Error
	return accounts, err
}

// FindByPhone 根据手机号查找账号
func (r *AccountRepository) FindByPhone(phone string) (*models.Account, error) {
	var account models.Account
	err := r.db.Where("phone = ?", phone).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// Update 更新账号
func (r *AccountRepository) Update(account *models.Account) error {
	return r.db.Save(account).Error
}

// Delete 删除账号
func (r *AccountRepository) Delete(id uint) error {
	return r.db.Delete(&models.Account{}, id).Error
}

// List 列出所有账号（管理员用）
func (r *AccountRepository) List(offset, limit int) ([]*models.Account, int64, error) {
	var accounts []*models.Account
	var total int64

	// 只查询未删除的账号
	query := r.db.Model(&models.Account{}).Where("deleted_at IS NULL")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("User").Offset(offset).Limit(limit).Find(&accounts).Error
	return accounts, total, err
}

// ListByUserID 列出指定用户的账号
func (r *AccountRepository) ListByUserID(userID uint, offset, limit int, phone string) ([]*models.Account, int64, error) {
	var accounts []*models.Account
	var total int64

	query := r.db.Model(&models.Account{}).Where("user_id = ?", userID)

	// 如果提供了手机号，添加模糊搜索条件
	if phone != "" {
		query = query.Where("phone LIKE ?", "%"+phone+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Offset(offset).Limit(limit).Find(&accounts).Error
	return accounts, total, err
}

// FindActiveAccounts 查找所有激活的账号
func (r *AccountRepository) FindActiveAccounts() ([]*models.Account, error) {
	var accounts []*models.Account
	err := r.db.Where("is_active = ?", true).Find(&accounts).Error
	return accounts, err
}

// FindActiveAccountsPaged 分页查找激活账号，用于 Worker/统计任务分批处理，避免一次性全表加载。
func (r *AccountRepository) FindActiveAccountsPaged(offset, limit int) ([]*models.Account, error) {
	var accounts []*models.Account
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	err := r.db.Where("is_active = ?", true).
		Order("id ASC").
		Offset(offset).
		Limit(limit).
		Find(&accounts).Error
	return accounts, err
}

// FindActiveAccountsByUserID 查找指定用户的所有激活账号
func (r *AccountRepository) FindActiveAccountsByUserID(userID uint) ([]*models.Account, error) {
	var accounts []*models.Account
	err := r.db.Where("user_id = ? AND is_active = ?", userID, true).Find(&accounts).Error
	return accounts, err
}

// UpdateCloudCount 更新云朵数量
func (r *AccountRepository) UpdateCloudCount(id uint, cloudCount int) error {
	return r.db.Model(&models.Account{}).Where("id = ?", id).Update("cloud_count", cloudCount).Error
}

// UpdateToken 更新Token
func (r *AccountRepository) UpdateToken(id uint, token string) error {
	return r.db.Model(&models.Account{}).Where("id = ?", id).Update("token", token).Error
}

// UpdateJWTToken 更新JWT Token
func (r *AccountRepository) UpdateJWTToken(id uint, jwtToken string) error {
	return r.db.Model(&models.Account{}).Where("id = ?", id).Update("jwt_token", jwtToken).Error
}

// UpdateAuthorizationFields 仅更新 authorization 刷新产生的字段，避免 Save 全量覆盖账号其他并发变更。
func (r *AccountRepository) UpdateAuthorizationFields(id uint, authValue, token, jwtToken, platform string, expireAt int64) error {
	updates := map[string]interface{}{
		"auth":            authValue,
		"token":           token,
		"expire_at":       expireAt,
		"jwt_error_count": 0,
	}
	if jwtToken != "" {
		updates["jwt_token"] = jwtToken
	}
	if platform != "" {
		updates["platform"] = platform
	}
	return r.db.Model(&models.Account{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// IncrementJWTErrorCount 原子自增 JWT 错误计数，并返回更新后的值。
func (r *AccountRepository) IncrementJWTErrorCount(id uint) (int, error) {
	result := r.db.Model(&models.Account{}).
		Where("id = ?", id).
		UpdateColumn("jwt_error_count", gorm.Expr("jwt_error_count + ?", 1))
	if result.Error != nil {
		return 0, result.Error
	}
	var account models.Account
	if err := r.db.Select("jwt_error_count").First(&account, id).Error; err != nil {
		return 0, err
	}
	return account.JWTErrorCount, nil
}

// ResetJWTErrorCount 原子重置 JWT 错误计数。
func (r *AccountRepository) ResetJWTErrorCount(id uint) error {
	return r.db.Model(&models.Account{}).
		Where("id = ? AND jwt_error_count > 0", id).
		UpdateColumn("jwt_error_count", 0).Error
}

// UpdateExpireAt 更新过期时间
func (r *AccountRepository) UpdateExpireAt(id uint, expireAt int64) error {
	return r.db.Model(&models.Account{}).Where("id = ?", id).Update("expire_at", expireAt).Error
}

// GetTotalCloudCountByUserID 获取用户所有账号的总云朵数
func (r *AccountRepository) GetTotalCloudCountByUserID(userID uint) (int, error) {
	var total int
	err := r.db.Model(&models.Account{}).Where("user_id = ?", userID).Select("COALESCE(SUM(cloud_count), 0)").Scan(&total).Error
	return total, err
}

// ExistsByPhone 检查手机号是否存在（全局检查，用于短信登录）
func (r *AccountRepository) ExistsByPhone(phone string) (bool, error) {
	var count int64
	err := r.db.Model(&models.Account{}).Where("phone = ?", phone).Count(&count).Error
	return count > 0, err
}

// ExistsByPhoneAndUserID 检查指定用户是否已存在该手机号
func (r *AccountRepository) ExistsByPhoneAndUserID(phone string, userID uint) (bool, error) {
	var count int64
	err := r.db.Model(&models.Account{}).Where("phone = ? AND user_id = ?", phone, userID).Count(&count).Error
	return count > 0, err
}

// FindByPhoneAndUserID 根据手机号和用户 ID 查找账号
// 未找到时返回 (nil, nil)，避免正常分支触发 record not found 日志。
func (r *AccountRepository) FindByPhoneAndUserID(phone string, userID uint) (*models.Account, error) {
	var account models.Account
	tx := r.db.Where("phone = ? AND user_id = ? AND deleted_at IS NULL", phone, userID).Limit(1).Find(&account)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, nil
	}
	return &account, nil
}

// SetActiveStatus 设置账号激活状态
func (r *AccountRepository) SetActiveStatus(id uint, isActive bool) error {
	return r.db.Model(&models.Account{}).Where("id = ?", id).Update("is_active", isActive).Error
}
