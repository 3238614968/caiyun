//go:build cgo

package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type adminCleanupFixture struct {
	db        *gorm.DB
	service   *AdminService
	admin     *models.User
	user      *models.User
	account   *models.Account
	rule      *models.ExchangeAccount
	task      *models.ExchangeTask
	operation *models.Operation
	session   *models.RefreshSession
}

func newAdminCleanupFixture(t *testing.T) *adminCleanupFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_foreign_keys=on", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Account{},
		&models.TaskLog{},
		&models.CloudStats{},
		&models.Product{},
		&models.ExchangeAccount{},
		&models.ExchangeTask{},
		&models.ExchangeRecord{},
		&models.WebSocketMessage{},
		&models.AuditLog{},
		&models.Operation{},
		&repository.ExchangeExecutionReceipt{},
		&models.RefreshSession{},
	); err != nil {
		t.Fatalf("auto migrate cleanup schema: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	admin := &models.User{Username: "admin-cleanup", Password: "hash", Role: "admin"}
	user := &models.User{Username: "user-cleanup", Password: "hash", Email: "user@example.com", Role: "user"}
	for _, candidate := range []*models.User{admin, user} {
		if err := userRepo.Create(candidate); err != nil {
			t.Fatalf("create user %q: %v", candidate.Username, err)
		}
	}
	for _, table := range []string{"task_logs", "exchange_records"} {
		if err := db.Exec("CREATE TABLE " + table + "_archive AS SELECT * FROM " + table + " WHERE 1 = 0").Error; err != nil {
			t.Fatalf("create %s archive: %v", table, err)
		}
		if err := db.Exec("INSERT INTO "+table+"_archive (id, user_id, message) VALUES (1, ?, ?), (2, ?, ?)", user.ID, "private-archive", admin.ID, "other-user-archive").Error; err != nil {
			t.Fatalf("seed %s archive: %v", table, err)
		}
	}
	account := &models.Account{UserID: user.ID, Phone: "13800138000", Auth: "secret-auth", Token: "secret-token", JWTToken: "secret-jwt", Remark: "private", IsActive: true}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("create account: %v", err)
	}
	product := &models.Product{PrizeID: "cleanup-prize", PrizeName: "cleanup product", IsActive: true}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	rule := &models.ExchangeAccount{UserID: user.ID, AccountID: account.ID, Phone: account.Phone, Auth: "rule-auth", Token: "rule-token", JWTToken: "rule-jwt", IsActive: true}
	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create exchange rule: %v", err)
	}
	task := &models.ExchangeTask{UserID: user.ID, ExchangeAccountID: rule.ID, ProductID: product.ID, PrizeID: product.PrizeID, PrizeName: product.PrizeName, Status: "pending"}
	if err := db.Create(task).Error; err != nil {
		t.Fatalf("create exchange task: %v", err)
	}
	if err := db.Create(&models.ExchangeRecord{UserID: user.ID, ExchangeAccountID: rule.ID, ExchangeTaskID: &task.ID, ProductID: product.ID, PrizeID: product.PrizeID, PrizeName: product.PrizeName, Status: "success", Message: "upstream-private"}).Error; err != nil {
		t.Fatalf("create exchange record: %v", err)
	}
	if err := db.Create(&models.TaskLog{UserID: user.ID, AccountID: account.ID, TaskType: "signin", Status: "success", Message: "private-log"}).Error; err != nil {
		t.Fatalf("create task log: %v", err)
	}
	if err := db.Create(&models.CloudStats{UserID: user.ID, AccountID: account.ID, Date: "2026-07-13", CloudCount: 100}).Error; err != nil {
		t.Fatalf("create cloud stats: %v", err)
	}
	if err := db.Create(&models.WebSocketMessage{UserID: user.ID, MessageID: "cleanup-message", Sequence: 1, Type: "notice", Data: "private-message"}).Error; err != nil {
		t.Fatalf("create websocket message: %v", err)
	}
	now := time.Now().UTC()
	operation := &models.Operation{ID: "00000000-0000-0000-0000-000000000001", UserID: user.ID, OperationType: models.OperationTypeAccountTask, Status: models.OperationQueued, AccountID: account.ID, Payload: `{"private":"payload"}`, IdempotencyKey: "cleanup-operation", QueuedAt: now}
	if err := db.Create(operation).Error; err != nil {
		t.Fatalf("create operation: %v", err)
	}
	monthly := &models.Operation{ID: "00000000-0000-0000-0000-000000000003", UserID: 0, OperationType: models.OperationTypeExchangeMonthly, IdempotencyKey: "system-monthly"}
	if err := db.Create(monthly).Error; err != nil {
		t.Fatal(err)
	}
	for _, businessUser := range []*models.User{user, admin} {
		receipt := &repository.ExchangeExecutionReceipt{OperationID: monthly.ID, UserID: businessUser.ID, ResourceID: businessUser.ID, ResourceKind: exchangeAccountReceiptKind}
		if err := repository.NewExchangeReceiptRepository(db).StartSubmission(receipt); err != nil {
			t.Fatal(err)
		}
	}
	session := &models.RefreshSession{ID: "00000000-0000-0000-0000-000000000002", UserID: user.ID, RefreshTokenHash: strings.Repeat("a", 64), ExpiresAt: now.Add(time.Hour)}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("create refresh session: %v", err)
	}
	if err := db.Create(&models.AuditLog{UserID: user.ID, Username: user.Username, Action: "TEST", Resource: "USER", IP: "127.0.0.1", UserAgent: "secret-agent", RequestData: "secret-request", ResponseData: "secret-response"}).Error; err != nil {
		t.Fatalf("create audit log: %v", err)
	}

	return &adminCleanupFixture{
		db:        db,
		service:   NewAdminService(userRepo, repository.NewAccountRepository(db), repository.NewTaskLogRepository(db), repository.NewTaskConfigRepository(db)),
		admin:     admin,
		user:      user,
		account:   account,
		rule:      rule,
		task:      task,
		operation: operation,
		session:   session,
	}
}

func assertRowCount(t *testing.T, db *gorm.DB, model interface{}, query string, args []interface{}, want int64) {
	t.Helper()
	var got int64
	if err := db.Unscoped().Model(model).Where(query, args...).Count(&got).Error; err != nil {
		t.Fatalf("count %T: %v", model, err)
	}
	if got != want {
		t.Fatalf("count %T = %d, want %d", model, got, want)
	}
}

func assertAdminArchiveRows(t *testing.T, f *adminCleanupFixture, wantTarget int64) {
	t.Helper()
	for _, table := range []string{"task_logs_archive", "exchange_records_archive"} {
		for userID, want := range map[uint]int64{f.user.ID: wantTarget, f.admin.ID: 1} {
			var got int64
			if err := f.db.Table(table).Where("user_id = ?", userID).Count(&got).Error; err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("%s rows for user %d = %d, want %d", table, userID, got, want)
			}
		}
	}
}

func TestAdminDeleteUserContextCleansOperationAndSessionAtomically(t *testing.T) {
	f := newAdminCleanupFixture(t)
	if err := f.service.DeleteUserContext(context.Background(), f.user.ID, f.admin.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	assertAdminArchiveRows(t, f, 0)
	assertRowCount(t, f.db, &repository.ExchangeExecutionReceipt{}, "user_id = ?", []interface{}{f.admin.ID}, 1)
	assertRowCount(t, f.db, &models.Operation{}, "idempotency_key = ?", []interface{}{"system-monthly"}, 1)

	for _, check := range []struct {
		model interface{}
		query string
		args  []interface{}
	}{
		{&models.Operation{}, "user_id = ?", []interface{}{f.user.ID}},
		{&repository.ExchangeExecutionReceipt{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.RefreshSession{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.ExchangeRecord{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.ExchangeTask{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.ExchangeAccount{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.TaskLog{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.CloudStats{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.Account{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.WebSocketMessage{}, "user_id = ?", []interface{}{f.user.ID}},
	} {
		assertRowCount(t, f.db, check.model, check.query, check.args, 0)
	}

	var deleted models.User
	if err := f.db.Unscoped().First(&deleted, f.user.ID).Error; err != nil {
		t.Fatalf("load deleted user: %v", err)
	}
	if !deleted.DeletedAt.Valid || deleted.Email != "" || deleted.NormalizedEmail != nil || deleted.Username == f.user.Username {
		t.Fatalf("user not safely anonymized: %#v", deleted)
	}
	var audit models.AuditLog
	if err := f.db.Where("user_id = ?", f.user.ID).First(&audit).Error; err != nil {
		t.Fatalf("load retained audit: %v", err)
	}
	if audit.Username != "deleted-user" || audit.IP != "" || audit.RequestData != "" || audit.ResponseData != "" {
		t.Fatalf("audit not anonymized: %#v", audit)
	}
}

func TestAdminDeleteUserContextRollsBackAllCleanup(t *testing.T) {
	f := newAdminCleanupFixture(t)
	trigger := fmt.Sprintf(`CREATE TRIGGER fail_audit_cleanup BEFORE UPDATE ON audit_logs WHEN OLD.user_id = %d BEGIN SELECT RAISE(ABORT, 'forced rollback'); END;`, f.user.ID)
	if err := f.db.Exec(trigger).Error; err != nil {
		t.Fatalf("create rollback trigger: %v", err)
	}
	if err := f.service.DeleteUserContext(context.Background(), f.user.ID, f.admin.ID); err == nil {
		t.Fatal("expected forced cleanup failure")
	}
	assertAdminArchiveRows(t, f, 1)
	assertRowCount(t, f.db, &repository.ExchangeExecutionReceipt{}, "user_id = ?", []interface{}{f.user.ID}, 1)

	for _, check := range []struct {
		model interface{}
		query string
		args  []interface{}
	}{
		{&models.Operation{}, "id = ?", []interface{}{f.operation.ID}},
		{&models.RefreshSession{}, "id = ?", []interface{}{f.session.ID}},
		{&models.ExchangeRecord{}, "user_id = ?", []interface{}{f.user.ID}},
		{&models.ExchangeTask{}, "id = ?", []interface{}{f.task.ID}},
		{&models.ExchangeAccount{}, "id = ?", []interface{}{f.rule.ID}},
		{&models.TaskLog{}, "account_id = ?", []interface{}{f.account.ID}},
		{&models.CloudStats{}, "account_id = ?", []interface{}{f.account.ID}},
		{&models.Account{}, "id = ?", []interface{}{f.account.ID}},
		{&models.WebSocketMessage{}, "user_id = ?", []interface{}{f.user.ID}},
	} {
		assertRowCount(t, f.db, check.model, check.query, check.args, 1)
	}
	var account models.Account
	if err := f.db.Unscoped().First(&account, f.account.ID).Error; err != nil {
		t.Fatalf("load rolled back account: %v", err)
	}
	if account.Auth != "secret-auth" || account.Token != "secret-token" || account.JWTToken != "secret-jwt" || account.DeletedAt.Valid {
		t.Fatalf("account cleanup escaped rollback: %#v", account)
	}
}

func TestAdminDeleteUserContextRollsBackWhenArchiveDeletionFails(t *testing.T) {
	f := newAdminCleanupFixture(t)
	trigger := fmt.Sprintf(`CREATE TRIGGER fail_archive_cleanup BEFORE DELETE ON task_logs_archive WHEN OLD.user_id = %d BEGIN SELECT RAISE(ABORT, 'forced archive failure'); END;`, f.user.ID)
	if err := f.db.Exec(trigger).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteUserContext(t.Context(), f.user.ID, f.admin.ID); err == nil {
		t.Fatal("user deletion must fail when private archive data cannot be erased")
	}
	assertAdminArchiveRows(t, f, 1)
	assertRowCount(t, f.db, &models.ExchangeRecord{}, "user_id = ?", []interface{}{f.user.ID}, 1)
	assertRowCount(t, f.db, &models.Operation{}, "user_id = ?", []interface{}{f.user.ID}, 1)
	var user models.User
	if err := f.db.First(&user, f.user.ID).Error; err != nil || user.DeletedAt.Valid {
		t.Fatalf("archive failure deleted user identity: user=%+v error=%v", user, err)
	}
}

func TestAdminDeleteAccountContextPreservesHistoryAndRollsBack(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		f := newAdminCleanupFixture(t)
		if err := f.service.DeleteAccountContext(t.Context(), f.account.ID); err != nil {
			t.Fatalf("delete account: %v", err)
		}
		for _, check := range []struct {
			model interface{}
			query string
			args  []interface{}
		}{
			{&models.Operation{}, "account_id = ?", []interface{}{f.account.ID}},
			{&models.ExchangeRecord{}, "exchange_rule_id = ?", []interface{}{f.rule.ID}},
			{&models.ExchangeTask{}, "exchange_rule_id = ?", []interface{}{f.rule.ID}},
			{&models.ExchangeAccount{}, "account_id = ?", []interface{}{f.account.ID}},
			{&models.TaskLog{}, "account_id = ?", []interface{}{f.account.ID}},
			{&models.CloudStats{}, "account_id = ?", []interface{}{f.account.ID}},
			{&models.Account{}, "id = ?", []interface{}{f.account.ID}},
		} {
			assertRowCount(t, f.db, check.model, check.query, check.args, 1)
		}
		var removed models.Account
		if err := f.db.Unscoped().First(&removed, f.account.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !removed.DeletedAt.Valid || removed.IsActive || removed.Auth != "" || removed.Token != "" || removed.JWTToken != "" {
			t.Fatalf("removed account retained active access: %+v", removed)
		}
		assertRowCount(t, f.db, &models.User{}, "id = ?", []interface{}{f.user.ID}, 1)
		assertRowCount(t, f.db, &models.RefreshSession{}, "id = ?", []interface{}{f.session.ID}, 1)
	})

	t.Run("rollback", func(t *testing.T) {
		f := newAdminCleanupFixture(t)
		trigger := fmt.Sprintf(`CREATE TRIGGER fail_account_cleanup BEFORE UPDATE ON accounts WHEN OLD.id = %d BEGIN SELECT RAISE(ABORT, 'forced rollback'); END;`, f.account.ID)
		if err := f.db.Exec(trigger).Error; err != nil {
			t.Fatalf("create rollback trigger: %v", err)
		}
		if err := f.service.DeleteAccountContext(t.Context(), f.account.ID); err == nil {
			t.Fatal("expected forced account cleanup failure")
		}
		assertRowCount(t, f.db, &models.Operation{}, "id = ?", []interface{}{f.operation.ID}, 1)
		assertRowCount(t, f.db, &models.ExchangeRecord{}, "exchange_rule_id = ?", []interface{}{f.rule.ID}, 1)
		assertRowCount(t, f.db, &models.ExchangeTask{}, "id = ?", []interface{}{f.task.ID}, 1)
		assertRowCount(t, f.db, &models.ExchangeAccount{}, "id = ?", []interface{}{f.rule.ID}, 1)
		assertRowCount(t, f.db, &models.TaskLog{}, "account_id = ?", []interface{}{f.account.ID}, 1)
		assertRowCount(t, f.db, &models.CloudStats{}, "account_id = ?", []interface{}{f.account.ID}, 1)
		assertRowCount(t, f.db, &models.Account{}, "id = ?", []interface{}{f.account.ID}, 1)
	})
}
