package repository

import (
	"caiyun/internal/models"

	"gorm.io/gorm"
)

type AccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) *AccountRepository {
	return &AccountRepository{db: db}
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

	err := query.Limit(limit).Find(&accounts).Error
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
