//go:build cgo
// +build cgo

package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMonthlyExchangeLockConflictIsNotReportedAsSuccess(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:monthly-lock?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&repository.ExchangeExecutionReceipt{}, &models.ExchangeRecord{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	service := NewExchangeService(nil, nil, nil, nil, repository.NewSystemConfigRepository(db), nil, nil, nil, nil)
	service.SetReceiptRepository(repository.NewExchangeReceiptRepository(db))
	service.SetLockStore(exchangeLockStoreStub{})
	if err := service.ExecuteMonthlyExchangeContext(WithTaskOperation(context.Background(), "monthly-lock-operation", 1), false); !errors.Is(err, ErrExchangeTaskConflict) {
		t.Fatalf("lock conflict returned %v", err)
	}
}

type exchangeLockStoreStub struct{}

func (exchangeLockStoreStub) SetNX(string, interface{}, time.Duration) (bool, error) {
	return false, nil
}
func (exchangeLockStoreStub) DelIfValue(string, string) (bool, error) { return true, nil }

func TestMonthlySubmissionFailureDoesNotCallUpstream(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared&_foreign_keys=on"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&repository.ExchangeExecutionReceipt{}, &models.ExchangeRecord{}, &models.Product{}); err != nil {
		t.Fatal(err)
	}
	service := NewExchangeService(repository.NewProductRepository(db), nil, nil, nil, nil, nil, nil, nil, nil)
	service.SetReceiptRepository(repository.NewExchangeReceiptRepository(db))
	account := &models.ExchangeAccount{ID: 1, UserID: 2, IsActive: true}
	ctx := WithTaskOperation(context.Background(), "missing-parent-operation", 1)
	result := service.executeMonthlyExchangeForAccountContext(ctx, account, "monthly", service.monthlySubmissionGuard(false))
	if result.outcome != models.ExchangeOutcomeNotSubmitted || !strings.Contains(result.message, "提交边界持久化失败") {
		t.Fatalf("invalid monthly result = %+v", result)
	}
}

func TestMonthlyReceiptRequiresRealOperationParent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared&_foreign_keys=on"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&repository.ExchangeExecutionReceipt{}); err != nil {
		t.Fatal(err)
	}
	repo := repository.NewExchangeReceiptRepository(db)
	service := &ExchangeService{exchangeReceiptRepo: repo}
	account := &models.ExchangeAccount{ID: 1, UserID: 7}
	product := &models.Product{ID: 9}
	if _, err := service.monthlyReceipt(context.Background(), account, product); !errors.Is(err, ErrExchangeInvalidInput) {
		t.Fatalf("missing operation returned %v", err)
	}
	operationID := uuid.NewString()
	ctx := WithTaskOperation(context.Background(), operationID, 1)
	receipt, err := service.monthlyReceipt(ctx, account, product)
	if err != nil || receipt.OperationID != operationID {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	// The GORM fixture now enforces the same operation FK as migration 024.
	if err := repo.StartSubmission(receipt); err == nil {
		t.Fatal("receipt without its durable parent was accepted")
	}
	if err := db.Create(&models.Operation{ID: operationID, UserID: 0, OperationType: models.OperationTypeExchangeMonthly,
		Status: models.OperationRunning, Payload: "{}", IdempotencyKey: "scheduled-monthly", QueuedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.StartSubmission(receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Outcome != string(models.ExchangeOutcomeUnknown) {
		t.Fatalf("submission boundary=%s", receipt.Outcome)
	}
	duplicate := &repository.ExchangeExecutionReceipt{OperationID: operationID, ResourceID: account.ID, ResourceKind: exchangeAccountReceiptKind, UserID: account.UserID, ProductID: product.ID}
	if err := repo.StartSubmission(duplicate); err == nil || duplicate.ID != 0 {
		t.Fatalf("duplicate was granted the submission: %+v err=%v", duplicate, err)
	}
}

func TestMonthlyFailedFinalizationStaysUnconfirmedOnRedelivery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared&_foreign_keys=on"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&repository.ExchangeExecutionReceipt{}, &models.ExchangeRecord{}, &models.Product{}); err != nil {
		t.Fatal(err)
	}
	operationID := uuid.NewString()
	if err := db.Create(&models.Operation{ID: operationID, UserID: 0, OperationType: models.OperationTypeExchangeMonthly, IdempotencyKey: operationID}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := WithTaskOperation(t.Context(), operationID, 1)
	service := &ExchangeService{productRepo: repository.NewProductRepository(db), exchangeReceiptRepo: repository.NewExchangeReceiptRepository(db)}
	account := &models.ExchangeAccount{ID: 3, UserID: 7}
	product := &models.Product{PrizeID: "monthly-local", PrizeName: "monthly"}
	if err := db.Create(product).Error; err != nil {
		t.Fatal(err)
	}
	receipt, err := service.monthlyReceipt(ctx, account, product)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.exchangeReceiptRepo.StartSubmission(receipt); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_record BEFORE INSERT ON exchange_records BEGIN SELECT RAISE(ABORT, 'record unavailable'); END`).Error; err != nil {
		t.Fatal(err)
	}
	result := service.finalizeMonthlyExchange(ctx, account, product, receipt, acceptedExchangeResult("accepted", 5), 5)
	if result.outcome != models.ExchangeOutcomeUnknown || !strings.Contains(result.message, "record unavailable") {
		t.Fatalf("reported an uncommitted result: %+v", result)
	}
	retry := service.executeMonthlyExchangeForAccountContext(ctx, account, product.PrizeID, service.monthlySubmissionGuard(false))
	stored, err := service.monthlyReceipt(ctx, account, product)
	if err != nil || retry.outcome != models.ExchangeOutcomeUnknown || stored.ID != receipt.ID || stored.Outcome != "unknown" || stored.CompletedAt != nil {
		t.Fatalf("redelivery overwrote an unresolved receipt: %+v, %+v, %v", retry, stored, err)
	}
	var records int64
	if err := db.Model(&models.ExchangeRecord{}).Count(&records).Error; err != nil || records != 0 {
		t.Fatalf("records=%d err=%v", records, err)
	}
}
