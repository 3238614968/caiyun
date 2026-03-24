package repository

import (
	"caiyun/internal/models"
	"context"
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

// FindAll 获取所有商品
func (r *ProductRepository) FindAll() ([]*models.Product, error) {
	var products []*models.Product
	err := r.db.Order("category ASC, p_order ASC").Find(&products).Error
	return products, err
}

// FindActive 获取所有启用的商品
func (r *ProductRepository) FindActive() ([]*models.Product, error) {
	var products []*models.Product
	err := r.db.Where("is_active = ?", true).
		Order("category ASC, p_order ASC").
		Find(&products).Error
	return products, err
}

// FindByCategory 根据分类获取商品
func (r *ProductRepository) FindByCategory(category string) ([]*models.Product, error) {
	var products []*models.Product
	err := r.db.Where("category = ? AND is_active = ?", category, true).
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
	err := r.db.Where("LOWER(prize_name) LIKE ? AND is_active = ?", searchTerm, true).
		Limit(limit).
		Order("p_order ASC").
		Find(&products).Error
	return products, err
}

// GetCategories 获取所有商品分类
func (r *ProductRepository) GetCategories() ([]string, error) {
	var categories []string
	err := r.db.Model(&models.Product{}).
		Where("is_active = ?", true).
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
	err := r.db.Model(&models.Product{}).Where("is_active = ?", true).Count(&count).Error
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
