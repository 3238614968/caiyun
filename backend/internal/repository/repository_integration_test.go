//go:build cgo
// +build cgo

package repository

import (
	"caiyun/internal/security"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"caiyun/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newRepositoryTestDB(t *testing.T, modelsToMigrate ...interface{}) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if len(modelsToMigrate) > 0 {
		if err := db.AutoMigrate(modelsToMigrate...); err != nil {
			t.Fatalf("auto migrate: %v", err)
		}
	}
	return db
}

func TestAccountRepositoryListByUserIDSortsActiveFirst(t *testing.T) {
	db := newRepositoryTestDB(t, &models.Account{})
	repo := NewAccountRepository(db)
	now := time.Now()
	accounts := []*models.Account{
		{UserID: 1, Phone: "inactive-high", CloudCount: 9999, IsActive: false, CreatedAt: now.Add(3 * time.Hour)},
		{UserID: 1, Phone: "active-low", CloudCount: 10, IsActive: true, CreatedAt: now.Add(1 * time.Hour)},
		{UserID: 1, Phone: "active-high", CloudCount: 100, IsActive: true, CreatedAt: now.Add(2 * time.Hour)},
		{UserID: 2, Phone: "other-user", CloudCount: 10000, IsActive: true, CreatedAt: now.Add(4 * time.Hour)},
	}
	for _, account := range accounts {
		if err := db.Create(account).Error; err != nil {
			t.Fatalf("create account: %v", err)
		}
	}

	got, total, err := repo.ListByUserID(1, 0, 10, "")
	if err != nil {
		t.Fatalf("ListByUserID error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	phones := []string{got[0].Phone, got[1].Phone, got[2].Phone}
	want := []string{"active-high", "active-low", "inactive-high"}
	if !reflect.DeepEqual(phones, want) {
		t.Fatalf("phones = %#v, want %#v", phones, want)
	}
}

func TestSystemConfigRepositoryBatchUpdateAndCanceledContext(t *testing.T) {
	db := newRepositoryTestDB(t, &models.SystemConfig{})
	repo := NewSystemConfigRepository(db)

	if err := repo.BatchUpdate([]SystemConfigUpdate{
		{Key: "exchange_enabled", Value: "true", Description: "enabled"},
		{Key: "exchange_time", Value: "10:00", Description: "time"},
	}); err != nil {
		t.Fatalf("BatchUpdate insert error: %v", err)
	}
	cfg, err := repo.GetByKey("exchange_enabled")
	if err != nil {
		t.Fatalf("GetByKey after insert error: %v", err)
	}
	if cfg.KeyValue != "true" || cfg.Description != "enabled" {
		t.Fatalf("config after insert = %+v", cfg)
	}

	if err := repo.BatchUpdate([]SystemConfigUpdate{{Key: "exchange_enabled", Value: "false", Description: "disabled"}}); err != nil {
		t.Fatalf("BatchUpdate update error: %v", err)
	}
	cfg, err = repo.GetByKey("exchange_enabled")
	if err != nil {
		t.Fatalf("GetByKey after update error: %v", err)
	}
	if cfg.KeyValue != "false" || cfg.Description != "disabled" {
		t.Fatalf("config after update = %+v", cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = repo.WithContext(ctx).BatchUpdate([]SystemConfigUpdate{{Key: "should_not_write", Value: "1"}})
	if err == nil {
		t.Fatalf("BatchUpdate with canceled context returned nil error")
	}
	if _, err := repo.GetByKey("should_not_write"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("canceled BatchUpdate wrote config or returned unexpected err: %v", err)
	}
}

func TestExchangeTaskRepositoryRunningStateMigration(t *testing.T) {
	db := newRepositoryTestDB(t, &models.ExchangeTask{})
	repo := NewExchangeTaskRepository(db)
	stale := &models.ExchangeTask{UserID: 1, ExchangeAccountID: 1, ProductID: 1, PrizeID: "p1", PrizeName: "stale", Status: string(models.ExchangeTaskRunning), UpdatedAt: time.Now().Add(-2 * time.Hour)}
	fresh := &models.ExchangeTask{UserID: 1, ExchangeAccountID: 1, ProductID: 2, PrizeID: "p2", PrizeName: "fresh", Status: string(models.ExchangeTaskRunning), UpdatedAt: time.Now()}
	pending := &models.ExchangeTask{UserID: 1, ExchangeAccountID: 1, ProductID: 3, PrizeID: "p3", PrizeName: "pending", Status: string(models.ExchangeTaskPending), UpdatedAt: time.Now().Add(-2 * time.Hour)}
	for _, task := range []*models.ExchangeTask{stale, fresh, pending} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task: %v", err)
		}
	}

	recovered, err := repo.RecoverStaleRunning(time.Hour)
	if err != nil {
		t.Fatalf("RecoverStaleRunning error: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}

	released, err := repo.ReleaseRunning(fresh.ID, "worker canceled")
	if err != nil || !released {
		t.Fatalf("ReleaseRunning() = (%v, %v), want (true, nil)", released, err)
	}
	released, err = repo.ReleaseRunning(fresh.ID, "must not overwrite terminal/pending state")
	if err != nil || released {
		t.Fatalf("second ReleaseRunning() = (%v, %v), want (false, nil)", released, err)
	}

	var tasks []models.ExchangeTask
	if err := db.Order("prize_id ASC").Find(&tasks).Error; err != nil {
		t.Fatalf("load tasks: %v", err)
	}
	statuses := map[string]string{}
	for _, task := range tasks {
		statuses[task.PrizeID] = task.Status
	}
	if statuses["p1"] != string(models.ExchangeTaskPending) || statuses["p2"] != string(models.ExchangeTaskPending) || statuses["p3"] != string(models.ExchangeTaskPending) {
		t.Fatalf("statuses = %#v", statuses)
	}
}

func TestTaskLogRepositoryFindByFilterEnforcesUserAndFilters(t *testing.T) {
	db := newRepositoryTestDB(t, &models.Account{}, &models.TaskLog{})
	repo := NewTaskLogRepository(db)
	accounts := []*models.Account{
		{ID: 1, UserID: 1, Phone: "13300000001", IsActive: true},
		{ID: 2, UserID: 2, Phone: "13300000002", IsActive: true},
	}
	for _, account := range accounts {
		if err := db.Create(account).Error; err != nil {
			t.Fatalf("create account: %v", err)
		}
	}
	logs := []*models.TaskLog{
		{UserID: 1, AccountID: 1, TaskType: "signin", Status: "success", Message: "ok", CreatedAt: time.Now()},
		{UserID: 1, AccountID: 1, TaskType: "shake", Status: "failed", Message: "failed", CreatedAt: time.Now()},
		{UserID: 2, AccountID: 2, TaskType: "signin", Status: "success", Message: "other", CreatedAt: time.Now()},
	}
	for _, item := range logs {
		if err := db.Create(item).Error; err != nil {
			t.Fatalf("create log: %v", err)
		}
	}

	accountID := uint(1)
	got, total, err := repo.FindByFilter(1, &accountID, "signin", "success", 0, 10)
	if err != nil {
		t.Fatalf("FindByFilter error: %v", err)
	}
	if total != 1 || len(got) != 1 {
		t.Fatalf("FindByFilter total=%d len=%d, want 1", total, len(got))
	}
	if got[0].UserID != 1 || got[0].AccountID != 1 || got[0].TaskType != "signin" || got[0].Status != "success" {
		t.Fatalf("unexpected log: %+v", got[0])
	}
	if got[0].Account.Phone != "13300000001" {
		t.Fatalf("preloaded account = %+v", got[0].Account)
	}
}

func TestExchangeRecordRepositoryStatsAreIndependent(t *testing.T) {
	db := newRepositoryTestDB(t, &models.ExchangeRecord{})
	repo := NewExchangeRecordRepository(db)
	now := time.Now()
	records := []*models.ExchangeRecord{
		{UserID: 1, ExchangeAccountID: 1, ProductID: 1, PrizeID: "p1", PrizeName: "a", Status: string(models.ExchangeRecordSuccess), Message: "ok", CreatedAt: now},
		{UserID: 1, ExchangeAccountID: 1, ProductID: 2, PrizeID: "p2", PrizeName: "b", Status: string(models.ExchangeRecordFailed), Message: "商品已兑完", CreatedAt: now},
		{UserID: 2, ExchangeAccountID: 2, ProductID: 3, PrizeID: "p3", PrizeName: "c", Status: string(models.ExchangeRecordFailed), Message: "other", CreatedAt: now},
	}
	for _, record := range records {
		if err := db.Create(record).Error; err != nil {
			t.Fatalf("create record: %v", err)
		}
	}

	success, failed, err := repo.GetStats(1, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetStats user error: %v", err)
	}
	if success != 1 || failed != 1 {
		t.Fatalf("user stats success=%d failed=%d, want 1/1", success, failed)
	}

	success, failed, err = repo.GetStats(0, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetStats global error: %v", err)
	}
	if success != 1 || failed != 2 {
		t.Fatalf("global stats success=%d failed=%d, want 1/2", success, failed)
	}
}

func TestExchangeTaskRepositoryGetByUserIDWithFilterSupportsKeywordCloudAndActive(t *testing.T) {
	db := newRepositoryTestDB(t,
		&models.User{},
		&models.Account{},
		&models.Product{},
		&models.ExchangeAccount{},
		&models.ExchangeTask{},
	)
	repo := NewExchangeTaskRepository(db)

	user1 := &models.User{Username: "repo-filter-u1", Password: "pwd", Email: "u1@example.com"}
	user2 := &models.User{Username: "repo-filter-u2", Password: "pwd", Email: "u2@example.com"}
	mustCreateRecords(t, db, user1, user2)

	accountA := &models.Account{UserID: user1.ID, Phone: "13900001001", Auth: "auth-a", CloudCount: 5200, Remark: "华东主账号", IsActive: true}
	accountB := &models.Account{UserID: user1.ID, Phone: "13900001002", Auth: "auth-b", CloudCount: 800, Remark: "江苏备用号", IsActive: false}
	accountC := &models.Account{UserID: user1.ID, Phone: "13900001003", Auth: "auth-c", CloudCount: 3200, Remark: "北京账号", IsActive: true}
	accountOther := &models.Account{UserID: user2.ID, Phone: "13900001999", Auth: "auth-other", CloudCount: 9999, Remark: "其他用户", IsActive: true}
	mustCreateRecords(t, db, accountA, accountB, accountC, accountOther)

	productA := &models.Product{PrizeID: "repo-filter-prod-a", PrizeName: "A", POrder: 100, Category: "会员"}
	productB := &models.Product{PrizeID: "repo-filter-prod-b", PrizeName: "B", POrder: 200, Category: "会员"}
	productC := &models.Product{PrizeID: "repo-filter-prod-c", PrizeName: "C", POrder: 300, Category: "会员"}
	productOther := &models.Product{PrizeID: "repo-filter-prod-other", PrizeName: "Other", POrder: 400, Category: "会员"}
	mustCreateRecords(t, db, productA, productB, productC, productOther)

	ruleA := &models.ExchangeAccount{UserID: user1.ID, AccountID: accountA.ID, Phone: accountA.Phone, Auth: "rule-auth-a", Remark: "上海主力规则", ExchangeTime1: "10:00:00", ExchangeTime2: "16:00:00", IsActive: true}
	ruleB := &models.ExchangeAccount{UserID: user1.ID, AccountID: accountB.ID, Phone: accountB.Phone, Auth: "rule-auth-b", Remark: "江苏备用规则", ExchangeTime1: "10:00:00", ExchangeTime2: "16:00:00", IsActive: true}
	ruleC := &models.ExchangeAccount{UserID: user1.ID, AccountID: accountC.ID, Phone: accountC.Phone, Auth: "rule-auth-c", Remark: "北京停用规则", ExchangeTime1: "10:00:00", ExchangeTime2: "16:00:00", IsActive: false}
	ruleOther := &models.ExchangeAccount{UserID: user2.ID, AccountID: accountOther.ID, Phone: accountOther.Phone, Auth: "rule-auth-other", Remark: "其他用户规则", ExchangeTime1: "10:00:00", ExchangeTime2: "16:00:00", IsActive: true}
	mustCreateRecords(t, db, ruleA, ruleB, ruleC, ruleOther)

	taskA := &models.ExchangeTask{UserID: user1.ID, ExchangeAccountID: ruleA.ID, ProductID: productA.ID, PrizeID: productA.PrizeID, PrizeName: productA.PrizeName, Status: string(models.ExchangeTaskPending), RestockCycle: "daily"}
	taskB := &models.ExchangeTask{UserID: user1.ID, ExchangeAccountID: ruleB.ID, ProductID: productB.ID, PrizeID: productB.PrizeID, PrizeName: productB.PrizeName, Status: string(models.ExchangeTaskPending), RestockCycle: "weekly"}
	taskC := &models.ExchangeTask{UserID: user1.ID, ExchangeAccountID: ruleC.ID, ProductID: productC.ID, PrizeID: productC.PrizeID, PrizeName: productC.PrizeName, Status: string(models.ExchangeTaskRunning), RestockCycle: "daily"}
	taskOther := &models.ExchangeTask{UserID: user2.ID, ExchangeAccountID: ruleOther.ID, ProductID: productOther.ID, PrizeID: productOther.PrizeID, PrizeName: productOther.PrizeName, Status: string(models.ExchangeTaskPending), RestockCycle: "daily"}
	mustCreateRecords(t, db, taskA, taskB, taskC, taskOther)

	t.Run("combined filters match active high cloud task", func(t *testing.T) {
		minCloud := 1000
		onlyActive := true
		got, err := repo.GetByUserIDWithFilter(user1.ID, ExchangeTaskFilter{
			AccountKeyword: "华东",
			Status:         string(models.ExchangeTaskPending),
			RestockCycle:   "daily",
			MinCloud:       &minCloud,
			OnlyActive:     &onlyActive,
		})
		if err != nil {
			t.Fatalf("GetByUserIDWithFilter combined error: %v", err)
		}
		if len(got) != 1 || got[0].ID != taskA.ID {
			t.Fatalf("combined filter got IDs = %#v, want [%d]", exchangeTaskIDs(got), taskA.ID)
		}
		if got[0].ExchangeAccount.AccountID != accountA.ID || got[0].ExchangeAccount.Account.Phone != accountA.Phone {
			t.Fatalf("combined filter preload mismatch: %+v", got[0].ExchangeAccount)
		}
	})

	t.Run("rule remark and max cloud match backup task", func(t *testing.T) {
		maxCloud := 1000
		got, err := repo.GetByUserIDWithFilter(user1.ID, ExchangeTaskFilter{
			Remark:   "备用规则",
			MaxCloud: &maxCloud,
		})
		if err != nil {
			t.Fatalf("GetByUserIDWithFilter remark/max error: %v", err)
		}
		if len(got) != 1 || got[0].ID != taskB.ID {
			t.Fatalf("remark/max filter got IDs = %#v, want [%d]", exchangeTaskIDs(got), taskB.ID)
		}
	})

	t.Run("only active excludes inactive account and inactive rule", func(t *testing.T) {
		onlyActive := true
		got, err := repo.GetByUserIDWithFilter(user1.ID, ExchangeTaskFilter{OnlyActive: &onlyActive})
		if err != nil {
			t.Fatalf("GetByUserIDWithFilter onlyActive error: %v", err)
		}
		if len(got) != 1 || got[0].ID != taskA.ID {
			t.Fatalf("onlyActive filter got IDs = %#v, want [%d]", exchangeTaskIDs(got), taskA.ID)
		}
	})
}

func TestExchangeTaskRepositoryGetTasksByTimeAtWithSkipsPersistsCalendarReason(t *testing.T) {
	db := newRepositoryTestDB(t,
		&models.User{},
		&models.Account{},
		&models.Product{},
		&models.ExchangeAccount{},
		&models.ExchangeTask{},
		&models.CalendarDate{},
	)
	repo := NewExchangeTaskRepository(db)

	user := &models.User{Username: "repo-schedule-u1", Password: "pwd", Email: "schedule@example.com"}
	mustCreateRecords(t, db, user)
	account := &models.Account{UserID: user.ID, Phone: "13900002001", Auth: "auth-schedule", CloudCount: 6000, Remark: "调度账号", IsActive: true}
	productA := &models.Product{PrizeID: "repo-schedule-prod-a", PrizeName: "A", POrder: 100, Category: "权益"}
	productB := &models.Product{PrizeID: "repo-schedule-prod-b", PrizeName: "B", POrder: 200, Category: "权益"}
	productC := &models.Product{PrizeID: "repo-schedule-prod-c", PrizeName: "C", POrder: 300, Category: "权益"}
	mustCreateRecords(t, db, account, productA, productB, productC)

	rule := &models.ExchangeAccount{UserID: user.ID, AccountID: account.ID, Phone: account.Phone, Auth: "rule-auth", Remark: "节假日规则", ExchangeTime1: "10:00:00", ExchangeTime2: "16:00:00", IsActive: true}
	mustCreateRecords(t, db, rule)

	now := time.Date(2026, time.June, 27, 10, 0, 0, 0, time.UTC)
	mismatchWeekday := (int(now.Weekday()) + 1) % 7
	calendarRow := &models.CalendarDate{Date: now.Format("2006-01-02"), DayType: "holiday", Name: "测试节假日", Source: "test"}
	workdayTask := &models.ExchangeTask{UserID: user.ID, ExchangeAccountID: rule.ID, ProductID: productA.ID, PrizeID: productA.PrizeID, PrizeName: productA.PrizeName, Status: string(models.ExchangeTaskPending), ScheduledExchangeTime: "10:00:00", RestockCycle: "daily", CalendarPolicy: "workday"}
	holidayTask := &models.ExchangeTask{UserID: user.ID, ExchangeAccountID: rule.ID, ProductID: productB.ID, PrizeID: productB.PrizeID, PrizeName: productB.PrizeName, Status: string(models.ExchangeTaskPending), ScheduledExchangeTime: "10:00:00", RestockCycle: "daily", CalendarPolicy: "holiday", SkipReason: "旧跳过原因"}
	weeklySkipTask := &models.ExchangeTask{UserID: user.ID, ExchangeAccountID: rule.ID, ProductID: productC.ID, PrizeID: productC.PrizeID, PrizeName: productC.PrizeName, Status: string(models.ExchangeTaskPending), ScheduledExchangeTime: "10:00:00", RestockCycle: "weekly", RestockWeekday: &mismatchWeekday, CalendarPolicy: "all"}
	mustCreateRecords(t, db, calendarRow, workdayTask, holidayTask, weeklySkipTask)

	runnable, skipped, err := repo.GetTasksByTimeAtWithSkips(now.Hour(), now.Minute(), now)
	if err != nil {
		t.Fatalf("GetTasksByTimeAtWithSkips error: %v", err)
	}
	if len(runnable) != 1 || runnable[0].ID != holidayTask.ID {
		t.Fatalf("runnable IDs = %#v, want [%d]", exchangeTaskIDs(runnable), holidayTask.ID)
	}
	if len(skipped) != 2 {
		t.Fatalf("len(skipped) = %d, want 2", len(skipped))
	}
	reasonByTaskID := map[uint]string{}
	for _, item := range skipped {
		reasonByTaskID[item.TaskID] = item.Reason
	}
	if !strings.Contains(reasonByTaskID[workdayTask.ID], "工作日") {
		t.Fatalf("workday skip reason = %q, want contains 工作日", reasonByTaskID[workdayTask.ID])
	}
	if !strings.Contains(reasonByTaskID[weeklySkipTask.ID], "补货周期为每周") {
		t.Fatalf("weekly skip reason = %q, want contains 补货周期为每周", reasonByTaskID[weeklySkipTask.ID])
	}

	var refreshedWorkday, refreshedHoliday, refreshedWeekly models.ExchangeTask
	if err := db.First(&refreshedWorkday, workdayTask.ID).Error; err != nil {
		t.Fatalf("reload workday task error: %v", err)
	}
	if err := db.First(&refreshedHoliday, holidayTask.ID).Error; err != nil {
		t.Fatalf("reload holiday task error: %v", err)
	}
	if err := db.First(&refreshedWeekly, weeklySkipTask.ID).Error; err != nil {
		t.Fatalf("reload weekly task error: %v", err)
	}
	if refreshedHoliday.SkipReason != "" {
		t.Fatalf("holiday task skip_reason = %q, want empty after runnable", refreshedHoliday.SkipReason)
	}
	if !strings.Contains(refreshedWorkday.SkipReason, "工作日") {
		t.Fatalf("persisted workday skip_reason = %q, want contains 工作日", refreshedWorkday.SkipReason)
	}
	if !strings.Contains(refreshedWeekly.SkipReason, "补货周期为每周") {
		t.Fatalf("persisted weekly skip_reason = %q, want contains 补货周期为每周", refreshedWeekly.SkipReason)
	}
}

func TestExchangeTaskRepositoryCalculateNextRunUsesCalendarOverride(t *testing.T) {
	db := newRepositoryTestDB(t,
		&models.CalendarDate{},
		&models.ExchangeAccount{},
		&models.ExchangeTask{},
	)
	repo := NewExchangeTaskRepository(db)

	from := time.Date(2026, time.June, 1, 9, 30, 0, 0, time.UTC)
	for from.Weekday() != time.Saturday {
		from = from.AddDate(0, 0, 1)
	}
	workdayOverride := &models.CalendarDate{Date: from.Format("2006-01-02"), DayType: "workday", Name: "调休工作日", Source: "test"}
	mustCreateRecords(t, db, workdayOverride)

	task := &models.ExchangeTask{
		CalendarPolicy:        "workday",
		ScheduledExchangeTime: "10:00:00",
		RestockCycle:          "daily",
		ExchangeAccount: models.ExchangeAccount{
			ExchangeTime1: "10:00:00",
			ExchangeTime2: "16:00:00",
		},
	}
	next := repo.CalculateNextRun(task, from)
	if next == nil {
		t.Fatalf("CalculateNextRun returned nil")
	}
	want := time.Date(from.Year(), from.Month(), from.Day(), 10, 0, 0, 0, from.Location())
	if !next.Equal(want) {
		t.Fatalf("next run = %s, want %s", next.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func mustCreateRecords(t *testing.T, db *gorm.DB, values ...interface{}) {
	t.Helper()
	for _, value := range values {
		if err := db.Create(value).Error; err != nil {
			t.Fatalf("create record %T error: %v", value, err)
		}
	}
}

func exchangeTaskIDs(tasks []*models.ExchangeTask) []uint {
	ids := make([]uint, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func TestExchangeAccountRepositoryCreateAndUpdatePersistEncryptedCredentials(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	security.ResetFieldCryptoForTests()
	defer security.ResetFieldCryptoForTests()

	db := newRepositoryTestDB(t, &models.ExchangeAccount{}, &models.ExchangeTask{}, &models.Product{})
	repo := NewExchangeAccountRepository(db)
	rule := &models.ExchangeAccount{
		UserID:        1,
		AccountID:     2,
		Phone:         "13900000000",
		Auth:          "plain-auth",
		Token:         "plain-token",
		JWTToken:      "plain-jwt",
		Remark:        "初始规则",
		ExchangeTime1: "10:00:00",
		ExchangeTime2: "16:00:00",
		IsActive:      true,
	}
	if err := repo.Create(rule); err != nil {
		t.Fatalf("Create error: %v", err)
	}

	var stored struct {
		Auth     string
		Token    string
		JWTToken string `gorm:"column:jwt_token"`
		Remark   string
	}
	if err := db.Model(&models.ExchangeAccount{}).Select("auth", "token", "jwt_token", "remark").Where("id = ?", rule.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("scan stored create row: %v", err)
	}
	if !security.IsEncryptedValue(stored.Auth) || !security.IsEncryptedValue(stored.Token) || !security.IsEncryptedValue(stored.JWTToken) {
		t.Fatalf("credentials should be encrypted in db: %+v", stored)
	}

	loaded, err := repo.GetByID(rule.ID)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if loaded.Auth != "plain-auth" || loaded.Token != "plain-token" || loaded.JWTToken != "plain-jwt" {
		t.Fatalf("loaded credentials = %+v, want plaintext values", loaded)
	}

	loaded.Remark = "更新规则"
	loaded.Auth = "next-auth"
	loaded.Token = "next-token"
	loaded.JWTToken = "next-jwt"
	if err := repo.Update(loaded); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	stored = struct {
		Auth     string
		Token    string
		JWTToken string `gorm:"column:jwt_token"`
		Remark   string
	}{}
	if err := db.Model(&models.ExchangeAccount{}).Select("auth", "token", "jwt_token", "remark").Where("id = ?", rule.ID).Scan(&stored).Error; err != nil {
		t.Fatalf("scan stored update row: %v", err)
	}
	if stored.Remark != "更新规则" {
		t.Fatalf("stored remark = %q, want 更新规则", stored.Remark)
	}
	if stored.Auth == "next-auth" || stored.Token == "next-token" || stored.JWTToken == "next-jwt" {
		t.Fatalf("updated credentials should remain encrypted in db: %+v", stored)
	}
}

func TestTaskLogRepositoryDateRangeQueriesClampUnsafeLimits(t *testing.T) {
	db := newRepositoryTestDB(t, &models.TaskLog{})
	repo := NewTaskLogRepository(db)
	now := time.Now()
	for i := 0; i < 220; i++ {
		item := &models.TaskLog{UserID: 1, AccountID: 1, TaskType: "signin", Status: "success", Message: "ok", CreatedAt: now.Add(time.Duration(i) * time.Second)}
		if err := db.Create(item).Error; err != nil {
			t.Fatalf("create log %d: %v", i, err)
		}
	}
	start := now.Add(-time.Minute)
	end := now.Add(221 * time.Second)

	logs, total, err := repo.FindByDateRange(start, end, -10, 0)
	if err != nil {
		t.Fatalf("FindByDateRange error: %v", err)
	}
	if total != 220 {
		t.Fatalf("total = %d, want 220", total)
	}
	if len(logs) != defaultTaskLogPageLimit {
		t.Fatalf("len(logs) = %d, want %d", len(logs), defaultTaskLogPageLimit)
	}

	logs, total, err = repo.FindByAccountIDAndDateRange(1, start, end, 0, maxTaskLogPageLimit+1)
	if err != nil {
		t.Fatalf("FindByAccountIDAndDateRange error: %v", err)
	}
	if total != 220 {
		t.Fatalf("account total = %d, want 220", total)
	}
	if len(logs) != defaultTaskLogPageLimit {
		t.Fatalf("account len(logs) = %d, want %d", len(logs), defaultTaskLogPageLimit)
	}
}

func TestExchangeTaskRepositoryUpdateTaskDefinitionDoesNotOverwriteCounters(t *testing.T) {
	db := newRepositoryTestDB(t, &models.ExchangeTask{})
	repo := NewExchangeTaskRepository(db)
	task := &models.ExchangeTask{
		UserID:            1,
		ExchangeAccountID: 1,
		ProductID:         1,
		PrizeID:           "old-prize",
		PrizeName:         "旧商品",
		TaskType:          string(models.ExchangeTaskFixed),
		MaxAttempts:       1,
		AttemptedCount:    3,
		SuccessCount:      2,
		FailCount:         1,
		Status:            string(models.ExchangeTaskPending),
	}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := repo.UpdateTaskDefinition(task.ID, 9, "new-prize", "新商品", string(models.ExchangeTaskLongTerm), 8, string(models.ExchangeTaskRunning)); err != nil {
		t.Fatalf("UpdateTaskDefinition error: %v", err)
	}

	updated, err := repo.GetByID(task.ID)
	if err != nil {
		t.Fatalf("GetByID error: %v", err)
	}
	if updated.ProductID != 9 || updated.PrizeID != "new-prize" || updated.PrizeName != "新商品" {
		t.Fatalf("updated snapshot = %+v", updated)
	}
	if updated.TaskType != string(models.ExchangeTaskLongTerm) || updated.MaxAttempts != 8 || updated.Status != string(models.ExchangeTaskRunning) {
		t.Fatalf("updated task definition = %+v", updated)
	}
	if updated.AttemptedCount != 3 || updated.SuccessCount != 2 || updated.FailCount != 1 {
		t.Fatalf("counters overwritten unexpectedly: attempted=%d success=%d failed=%d", updated.AttemptedCount, updated.SuccessCount, updated.FailCount)
	}
}

func TestCloudStatsRepositoryFindByDateRangeAppliesPaging(t *testing.T) {
	db := newRepositoryTestDB(t, &models.Account{}, &models.CloudStats{})
	repo := NewCloudStatsRepository(db)
	account := &models.Account{ID: 1, UserID: 1, Phone: "13300000000", IsActive: true}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	rows := []*models.CloudStats{
		{UserID: 1, AccountID: 1, Date: "2026-06-25", CloudCount: 100},
		{UserID: 1, AccountID: 1, Date: "2026-06-26", CloudCount: 200},
		{UserID: 1, AccountID: 1, Date: "2026-06-27", CloudCount: 300},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create cloud stats: %v", err)
		}
	}

	stats, total, err := repo.FindByDateRange("2026-06-25", "2026-06-27", -10, 2)
	if err != nil {
		t.Fatalf("FindByDateRange error: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(stats) != 2 {
		t.Fatalf("len(stats) = %d, want 2", len(stats))
	}
	if stats[0].Date != "2026-06-27" || stats[1].Date != "2026-06-26" {
		t.Fatalf("dates = %s, %s; want latest two rows", stats[0].Date, stats[1].Date)
	}
}
