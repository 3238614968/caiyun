//go:build cgo

package worker

import (
	"context"
	"errors"
	"testing"

	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/repository"
	"caiyun/internal/scheduler"
	"caiyun/internal/services"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type monthlyDispatchQueue struct {
	queue.ReliableTaskQueue
	calls int
}

func (queued *monthlyDispatchQueue) EnqueueMessage(*queue.TaskMessage) error {
	queued.calls++
	return nil
}

func TestMonthlyCronOnlyDispatchesWhenExplicitlyEnabled(t *testing.T) {
	for _, test := range []struct {
		name, value string
		readFails   bool
		want        int64
	}{
		{name: "missing"}, {name: "query error", readFails: true}, {name: "disabled", value: "false"},
		{name: "unrecognized", value: "unexpected"}, {name: "enabled", value: "true", want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.AutoMigrate(&models.SystemConfig{}, &models.Operation{}); err != nil {
				t.Fatal(err)
			}
			configs := repository.NewSystemConfigRepository(db)
			jobs := scheduler.NewScheduler()
			defer jobs.Stop()
			queued := &monthlyDispatchQueue{}
			registerMonthlyExchangeJob(jobs, services.NewOperationService(repository.NewOperationRepository(db), queued), configs)
			if test.value != "" {
				if err := configs.Set("exchange_monthly_enabled", test.value); err != nil {
					t.Fatal(err)
				}
			}
			if test.readFails {
				if err := db.Migrator().DropTable(&models.SystemConfig{}); err != nil {
					t.Fatal(err)
				}
			}
			result, err := jobs.ExecuteJob("monthly_exchange")
			if err != nil || (result.Error != nil) != test.readFails {
				t.Fatalf("unexpected cron result: %+v err=%v", result, err)
			}
			var rows int64
			if err := db.Model(&models.Operation{}).Count(&rows).Error; err != nil || rows != test.want || int64(queued.calls) != test.want {
				t.Fatalf("operations=%d dispatches=%d want=%d err=%v", rows, queued.calls, test.want, err)
			}
		})
	}
}

func TestMonthlyWorkerCarriesOperationContextForAPIAndCron(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ExchangeAccount{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	configs := repository.NewSystemConfigRepository(db)
	exchange := services.NewExchangeService(nil, repository.NewExchangeAccountRepository(db), nil, nil, configs, nil, nil, nil, nil)
	w := &Worker{ctx: context.Background(), exchangeService: exchange}
	for _, userID := range []uint{7, 0} {
		// Empty account sets complete locally. Without the operation context,
		// the service rejects the call before querying accounts.
		operation := &models.Operation{ID: uuid.NewString(), UserID: userID, OperationType: models.OperationTypeExchangeMonthly, AttemptCount: 1}
		if userID == 0 {
			if err := w.executeOperation(operation); !errors.Is(err, services.ErrExchangeTaskConflict) {
				t.Fatalf("scheduled execution bypassed missing automatic switch: %v", err)
			}
			if err := configs.Set("exchange_monthly_enabled", "true"); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.executeOperation(operation); err != nil {
			t.Fatalf("user_id=%d monthly execution: %v", userID, err)
		}
	}
}
