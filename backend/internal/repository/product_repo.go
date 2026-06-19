package repository

import (
	"caiyun/internal/models"
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ProductRepository 商品数据访问层
type ProductRepository struct {
	db *gorm.DB
}

func NewProductRepository(db *gorm.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *ProductRepository) WithContext(ctx context.Context) *ProductRepository {
	if ctx == nil {
		return r
	}
	return &ProductRepository{db: r.db.WithContext(ctx)}
}

// Create 创建商品
func (r *ProductRepository) Create(product *models.Product) error {
	return r.db.Create(product).Error
}

// Update 更新商品
func (r *ProductRepository) Update(product *models.Product) error {
	return r.db.Save(product).Error
}

// Delete 删除商品
func (r *ProductRepository) Delete(id uint) error {
	return r.db.Delete(&models.Product{}, id).Error
}

// GetByID 根据 ID 获取商品
func (r *ProductRepository) GetByID(id uint) (*models.Product, error) {
	var product models.Product
	err := r.db.First(&product, id).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

// GetByPrizeID 根据 PrizeID 获取商品
func (r *ProductRepository) GetByPrizeID(prizeID string) (*models.Product, error) {
	var product models.Product
	err := r.db.Where("prize_id = ?", prizeID).First(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

// FindExchangeableByName 根据商品名查找当前可用于兑换接口的商品。
// 历史任务可能保存了 memo(JSON) 作为 prize_id，这里优先返回最新的真实 prize_id。
func (r *ProductRepository) FindExchangeableReplacement(name string, legacyPrizeID string) (*models.Product, error) {
	var product models.Product
	if strings.TrimSpace(legacyPrizeID) != "" {
		err := r.db.Where("memo = ? AND is_active = ? AND is_deleted = ?", legacyPrizeID, true, false).
			Where("prize_id <> '' AND prize_id NOT LIKE ?", "{%").
			Order("daily_remainder_count DESC, updated_at DESC").
			First(&product).Error
		if err == nil {
			return &product, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	err := r.db.Where("prize_name = ? AND is_active = ? AND is_deleted = ?", name, true, false).
		Where("prize_id <> '' AND prize_id NOT LIKE ?", "{%").
		Order("daily_remainder_count DESC, updated_at DESC").
		First(&product).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &product, nil
}

// FindAll 获取所有商品
func (r *ProductRepository) FindAll() ([]*models.Product, error) {
	var products []*models.Product
	err := r.db.Order("category ASC, p_order ASC").Find(&products).Error
	return products, err
}

// FindActive 获取所有启用的商品
func (r *ProductRepository) FindActive() ([]*models.Product, error) {
	var products []*models.Product
	err := r.db.Where("is_active = ? AND is_deleted = ?", true, false).
		Order("category ASC, p_order ASC").
		Find(&products).Error
	return products, err
}

// FindByCategory 根据分类获取商品
func (r *ProductRepository) FindByCategory(category string) ([]*models.Product, error) {
	var products []*models.Product
	err := r.db.Where("category = ? AND is_active = ? AND is_deleted = ?", category, true, false).
		Order("p_order ASC").
		Find(&products).Error
	return products, err
}

// Search 搜索商品 (模糊匹配商品名称)
func (r *ProductRepository) Search(keyword string, limit int) ([]*models.Product, error) {
	if limit <= 0 {
		limit = 20
	}

	var products []*models.Product
	searchTerm := "%" + strings.ToLower(keyword) + "%"
	err := r.db.Where("LOWER(prize_name) LIKE ? AND is_active = ? AND is_deleted = ?", searchTerm, true, false).
		Limit(limit).
		Order("p_order ASC").
		Find(&products).Error
	return products, err
}

// GetCategories 获取所有商品分类
func (r *ProductRepository) GetCategories() ([]string, error) {
	var categories []string
	err := r.db.Model(&models.Product{}).
		Where("is_active = ? AND is_deleted = ?", true, false).
		Distinct().
		Pluck("category", &categories).Error
	return categories, err
}

// BatchCreate 批量创建商品
func (r *ProductRepository) BatchCreate(products []*models.Product) error {
	return r.db.CreateInBatches(products, 100).Error
}

// BatchUpdateByPrizeID 根据 PrizeID 批量更新商品
func (r *ProductRepository) BatchUpdateByPrizeID(products []*models.Product) error {
	ctx := context.Background()
	for _, product := range products {
		if err := r.db.WithContext(ctx).
			Where("prize_id = ?", product.PrizeID).
			Updates(map[string]interface{}{
				"prize_name":            product.PrizedName,
				"p_order":               product.POrder,
				"category":              product.Category,
				"daily_remainder_count": product.DailyRemainderCount,
				"memo":                  product.Memo,
				"is_active":             product.IsActive,
				"updated_at":            time.Now(),
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

// Upsert 插入或更新商品 (如果存在则更新，不存在则插入)
func (r *ProductRepository) Upsert(product *models.Product) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "prize_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"prize_name", "p_order", "category", "daily_remainder_count", "memo", "is_active"}),
	}).Create(product).Error
}

// Count 获取商品总数
func (r *ProductRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&models.Product{}).Where("is_active = ? AND is_deleted = ?", true, false).Count(&count).Error
	return count, err
}

// UpsertProducts 批量 UPSERT 商品（返回更新、插入、删除数量）
func (r *ProductRepository) UpsertProducts(products []*models.Product) (updated, inserted, deleted int, err error) {
	ctx := context.Background()
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 获取当前数据库中所有商品的 PrizeID
	var existingPrizeIDs []string
	err = tx.Model(&models.Product{}).Pluck("prize_id", &existingPrizeIDs).Error
	if err != nil {
		tx.Rollback()
		return 0, 0, 0, err
	}

	existingMap := make(map[string]bool)
	for _, id := range existingPrizeIDs {
		existingMap[id] = true
	}

	// 分离需要插入和更新的商品
	var toInsert []*models.Product
	var toUpdate []*models.Product

	for _, product := range products {
		if _, exists := existingMap[product.PrizeID]; exists {
			toUpdate = append(toUpdate, product)
		} else {
			toInsert = append(toInsert, product)
		}
	}

	// 批量插入新商品
	if len(toInsert) > 0 {
		if err := tx.CreateInBatches(toInsert, 100).Error; err != nil {
			tx.Rollback()
			return 0, 0, 0, err
		}
		inserted = len(toInsert)
	}

	// 批量更新已有商品
	if len(toUpdate) > 0 {
		for _, product := range toUpdate {
			if err := tx.Model(&models.Product{}).Where("prize_id = ?", product.PrizeID).Updates(map[string]interface{}{
				"prize_name":            product.PrizedName,
				"p_order":               product.POrder,
				"category":              product.Category,
				"daily_limit_count":     product.DailyLimitCount,
				"daily_count":           product.DailyCount,
				"daily_remainder_count": product.DailyRemainderCount,
				"image_url":             product.ImageURL,
				"stock_status":          product.StockStatus,
				"last_stock_check":      product.LastStockCheck,
				"is_deleted":            product.IsDeleted,
				"updated_at":            time.Now(),
			}).Error; err != nil {
				tx.Rollback()
				return updated, inserted, 0, err
			}
			updated++
		}
	}

	// 标记下架商品（API 中没有但数据库有的）
	apiPrizeIDs := make([]string, 0, len(products))
	for _, product := range products {
		apiPrizeIDs = append(apiPrizeIDs, product.PrizeID)
	}

	// 找出 API 中没有的商品，标记为已删除
	var toDeleteIDs []string
	err = tx.Model(&models.Product{}).Where("prize_id NOT IN ? AND is_deleted = ?", apiPrizeIDs, false).Pluck("id", &toDeleteIDs).Error
	if err == nil && len(toDeleteIDs) > 0 {
		result := tx.Model(&models.Product{}).Where("id IN ?", toDeleteIDs).Update("is_deleted", true)
		if result.Error == nil {
			deleted = int(result.RowsAffected)
		}
	}

	// 提交事务
	tx.Commit()
	if tx.Error != nil {
		return updated, inserted, deleted, tx.Error
	}

	return updated, inserted, deleted, nil
}

// ReplaceProducts 用本次成功拉取到的商品列表整体替换本地商品缓存。
// 注意：只有上游商品列表完整获取并解析成功后才调用此方法；如果上游失败，调用方不应调用，
// 从而继续使用上一次缓存。历史商品不会物理删除，避免破坏兑换记录外键，但会标记为不可用。
func (r *ProductRepository) ReplaceProducts(products []*models.Product) (updated, inserted, disabled, syncedTasks, stoppedTasks int, err error) {
	if len(products) == 0 {
		return 0, 0, 0, 0, 0, nil
	}

	ctx := context.Background()
	tx := r.db.WithContext(ctx).Begin()
	defer func() {
		if v := recover(); v != nil {
			tx.Rollback()
		}
	}()

	var existingPrizeIDs []string
	if err = tx.Model(&models.Product{}).Pluck("prize_id", &existingPrizeIDs).Error; err != nil {
		tx.Rollback()
		return 0, 0, 0, 0, 0, err
	}
	existingMap := make(map[string]bool, len(existingPrizeIDs))
	for _, id := range existingPrizeIDs {
		existingMap[id] = true
	}

	now := time.Now()
	result := tx.Model(&models.Product{}).
		Where("is_active = ? OR is_deleted = ?", true, false).
		Updates(map[string]interface{}{
			"is_active":  false,
			"is_deleted": true,
			"updated_at": now,
		})
	if result.Error != nil {
		tx.Rollback()
		return 0, 0, 0, 0, 0, result.Error
	}
	disabled = int(result.RowsAffected)

	for _, product := range products {
		product.IsActive = true
		product.IsDeleted = false
		if product.UpdatedAt.IsZero() {
			product.UpdatedAt = now
		}
		if existingMap[product.PrizeID] {
			updated++
		} else {
			inserted++
		}
	}

	if err = tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "prize_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"prize_name",
			"p_order",
			"category",
			"daily_remainder_count",
			"daily_limit_count",
			"daily_count",
			"image_url",
			"stock_status",
			"last_stock_check",
			"memo",
			"is_active",
			"is_deleted",
			"updated_at",
		}),
	}).CreateInBatches(products, 100).Error; err != nil {
		tx.Rollback()
		return 0, 0, 0, 0, 0, err
	}

	var latestProducts []*models.Product
	if err = tx.Where("is_active = ? AND is_deleted = ?", true, false).
		Order("daily_remainder_count DESC, updated_at DESC").
		Find(&latestProducts).Error; err != nil {
		tx.Rollback()
		return 0, 0, 0, 0, 0, err
	}

	syncedTasks, stoppedTasks, err = syncExchangeTasksToLatestProducts(tx, latestProducts, now)
	if err != nil {
		tx.Rollback()
		return 0, 0, 0, 0, 0, err
	}

	if err = tx.Commit().Error; err != nil {
		return 0, 0, 0, 0, 0, err
	}

	return updated, inserted, disabled, syncedTasks, stoppedTasks, nil
}

func syncExchangeTasksToLatestProducts(tx *gorm.DB, products []*models.Product, now time.Time) (synced int, stopped int, err error) {
	if len(products) == 0 {
		return 0, 0, nil
	}

	seenName := make(map[string]bool)
	for _, product := range products {
		if product == nil || product.ID == 0 || product.PrizeID == "" {
			continue
		}

		if strings.TrimSpace(product.Memo) != "" {
			result := tx.Model(&models.ExchangeTask{}).
				Where("deleted_at IS NULL").
				Where("status IN ?", []string{string(models.ExchangeTaskPending), string(models.ExchangeTaskRunning)}).
				Where("prize_id = ?", product.Memo).
				Updates(map[string]interface{}{
					"product_id": product.ID,
					"prize_id":   product.PrizeID,
					"prize_name": product.PrizedName,
					"updated_at": now,
				})
			if result.Error != nil {
				return synced, stopped, result.Error
			}
			synced += int(result.RowsAffected)
		}

		if product.PrizedName == "" || seenName[product.PrizedName] {
			continue
		}
		seenName[product.PrizedName] = true

		result := tx.Model(&models.ExchangeTask{}).
			Where("deleted_at IS NULL").
			Where("status IN ?", []string{string(models.ExchangeTaskPending), string(models.ExchangeTaskRunning)}).
			Where("prize_name = ?", product.PrizedName).
			Updates(map[string]interface{}{
				"product_id": product.ID,
				"prize_id":   product.PrizeID,
				"prize_name": product.PrizedName,
				"updated_at": now,
			})
		if result.Error != nil {
			return synced, stopped, result.Error
		}
		synced += int(result.RowsAffected)
	}

	result := tx.Exec(`
UPDATE exchange_tasks AS t
LEFT JOIN products AS p
  ON p.prize_name = t.prize_name
 AND p.is_active = TRUE
 AND p.is_deleted = FALSE
 AND p.deleted_at IS NULL
SET t.status = ?,
    t.last_result = ?,
    t.updated_at = ?
WHERE t.deleted_at IS NULL
  AND t.status IN ?
  AND p.id IS NULL
`, string(models.ExchangeTaskCompleted), "商品已下架或不存在，已根据最新商品列表停止任务", now, []string{string(models.ExchangeTaskPending), string(models.ExchangeTaskRunning)})
	if result.Error != nil {
		return synced, stopped, result.Error
	}
	stopped = int(result.RowsAffected)

	return synced, stopped, nil
}
