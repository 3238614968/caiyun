package services

import (
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/ws"
	"context"
)

// ExchangeService 抢兑服务
type ExchangeService struct {
	productRepo         *repository.ProductRepository
	exchangeAccountRepo *repository.ExchangeAccountRepository
	exchangeTaskRepo    *repository.ExchangeTaskRepository
	accountRepo         *repository.AccountRepository
	configRepo          *repository.SystemConfigRepository
	exchangeRecordRepo  *repository.ExchangeRecordRepository
	taskLogRepo         *repository.TaskLogRepository
	authMgr             *auth.Auth
	tokenMgr            *TokenManager
	hub                 *ws.Hub
	lockStore           exchangeLockStore
	unitOfWork          repository.UnitOfWork
}

// exchangeLockStore and schedulerLeaseStore share the same fencing-safe Redis
// primitive. Keeping this alias avoids divergent lock semantics between the
// API/Worker execution paths.
type exchangeLockStore = schedulerLeaseStore

func NewExchangeService(
	productRepo *repository.ProductRepository,
	exchangeAccountRepo *repository.ExchangeAccountRepository,
	exchangeTaskRepo *repository.ExchangeTaskRepository,
	accountRepo *repository.AccountRepository,
	configRepo *repository.SystemConfigRepository,
	exchangeRecordRepo *repository.ExchangeRecordRepository,
	taskLogRepo *repository.TaskLogRepository,
	authMgr *auth.Auth,
	tokenMgr *TokenManager,
	eventHubs ...*ws.Hub,
) *ExchangeService {
	var eventHub *ws.Hub
	if len(eventHubs) > 0 {
		eventHub = eventHubs[0]
	}
	return &ExchangeService{
		productRepo:         productRepo,
		exchangeAccountRepo: exchangeAccountRepo,
		exchangeTaskRepo:    exchangeTaskRepo,
		accountRepo:         accountRepo,
		configRepo:          configRepo,
		exchangeRecordRepo:  exchangeRecordRepo,
		taskLogRepo:         taskLogRepo,
		authMgr:             authMgr,
		tokenMgr:            tokenMgr,
		hub:                 eventHub,
		unitOfWork:          repository.NewUnitOfWorkFromExchangeAccountRepository(exchangeAccountRepo),
	}
}

// SetEventHub attaches the Hub owned by the current process. It is optional
// for focused service tests, where business persistence remains independent
// from realtime delivery.
func (s *ExchangeService) SetEventHub(eventHub *ws.Hub) {
	if s != nil {
		s.hub = eventHub
	}
}

func (s *ExchangeService) SetLockStore(lockStore exchangeLockStore) {
	s.lockStore = lockStore
}

// SetUnitOfWork overrides the default repository-derived Unit of Work. This is
// primarily useful for composition roots and deterministic failure tests.
func (s *ExchangeService) SetUnitOfWork(unitOfWork repository.UnitOfWork) {
	if s == nil || unitOfWork == nil {
		return
	}
	s.unitOfWork = unitOfWork
}

// UpdateProducts 更新商品信息 (从云盘 API 获取)
func (s *ExchangeService) UpdateProducts(accountID uint) error {
	return s.UpdateProductsContext(context.Background(), accountID)
}

func (s *ExchangeService) UpdateProductsContext(ctx context.Context, accountID uint) error {
	_, err := syncProductsFromCloudContext(ctx, s.productRepo, s.accountRepo, accountID)
	return err
}

// SearchProducts 搜索商品
func (s *ExchangeService) SearchProducts(keyword string, limit int) ([]*models.Product, error) {
	return s.SearchProductsContext(context.Background(), keyword, limit)
}

func (s *ExchangeService) SearchProductsContext(ctx context.Context, keyword string, limit int) ([]*models.Product, error) {
	repo := s.productRepo.WithContext(ctx)
	if keyword == "" {
		return repo.FindActive()
	}
	return repo.Search(keyword, limit)
}

// GetProductCategories 获取商品分类
func (s *ExchangeService) GetProductCategories() ([]string, error) {
	return s.GetProductCategoriesContext(context.Background())
}

func (s *ExchangeService) GetProductCategoriesContext(ctx context.Context) ([]string, error) {
	return s.productRepo.WithContext(ctx).GetCategories()
}
