package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"

	"caiyun/internal/security"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The database/sql fixture executes the real GORM query/preload/AfterFind
// pipeline without requiring SQLite's CGO driver. Stored credential values
// are deliberately unreadable, reproducing the production upgrade failure.
type metadataFixtureConnector struct{}
type metadataFixtureDriver struct{}
type metadataFixtureConn struct{}
type metadataFixtureRows struct {
	columns []string
	values  []driver.Value
	read    bool
}

func (metadataFixtureConnector) Connect(context.Context) (driver.Conn, error) {
	return metadataFixtureConn{}, nil
}
func (metadataFixtureConnector) Driver() driver.Driver { return metadataFixtureDriver{} }
func (metadataFixtureDriver) Open(string) (driver.Conn, error) {
	return metadataFixtureConn{}, nil
}
func (metadataFixtureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("fixture requires QueryContext")
}
func (metadataFixtureConn) Close() error { return nil }
func (metadataFixtureConn) Begin() (driver.Tx, error) {
	return nil, errors.New("read-only fixture")
}
func (r *metadataFixtureRows) Columns() []string { return r.columns }
func (r *metadataFixtureRows) Close() error      { return nil }
func (r *metadataFixtureRows) Next(dst []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	copy(dst, r.values)
	return nil
}

var fixtureTablePattern = regexp.MustCompile("(?i)FROM\\s+`?(\\w+)`?")

func (metadataFixtureConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(strings.ToUpper(query), "COUNT(*)") {
		return &metadataFixtureRows{columns: []string{"count"}, values: []driver.Value{int64(1)}}, nil
	}
	match := fixtureTablePattern.FindStringSubmatch(query)
	if len(match) != 2 {
		return nil, fmt.Errorf("unexpected fixture query: %s", query)
	}
	row := map[string]driver.Value{"id": int64(1), "user_id": int64(7), "account_id": int64(1)}
	var all []string
	switch match[1] {
	case "accounts", "exchange_rules":
		row["phone"], row["remark"], row["is_active"] = "fixture-phone", "fixture-remark", true
		row["auth"], row["token"], row["jwt_token"] = "enc:v1:ZmFrZQ", "enc:v1:ZmFrZQ", "enc:v1:ZmFrZQ"
		all = []string{"id", "user_id", "account_id", "phone", "remark", "is_active", "auth", "token", "jwt_token"}
	case "task_logs":
		row["task_type"], row["status"], row["message"] = "signin", "success", "fixture-log"
		all = []string{"id", "user_id", "account_id", "task_type", "status", "message"}
	case "exchange_tasks":
		row["exchange_rule_id"], row["product_id"], row["status"] = int64(1), int64(1), "pending"
		all = []string{"id", "user_id", "exchange_rule_id", "product_id", "status"}
	case "products":
		row["prize_name"] = "fixture-product"
		all = []string{"id", "prize_name"}
	case "users":
		row["id"], row["username"] = int64(7), "fixture-owner"
		all = []string{"id", "username"}
	default:
		return nil, fmt.Errorf("unexpected fixture table: %s", match[1])
	}
	projection := strings.TrimSpace(query[len("SELECT "):strings.Index(strings.ToUpper(query), " FROM ")])
	columns := all
	if projection != "*" && !strings.HasSuffix(projection, ".*") {
		columns = nil
		for _, field := range strings.Split(projection, ",") {
			parts := strings.Split(strings.TrimSpace(field), ".")
			columns = append(columns, strings.Trim(parts[len(parts)-1], "`"))
		}
	}
	values := make([]driver.Value, len(columns))
	for i, column := range columns {
		values[i] = row[column]
	}
	return &metadataFixtureRows{columns: columns, values: values}, nil
}

func newMetadataFixtureDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	t.Setenv("DATA_ENCRYPTION_KEYS", "v1=0123456789abcdef0123456789abcdef")
	t.Setenv("DATA_ENCRYPTION_CURRENT_VERSION", "v1")
	t.Setenv("FIELD_CRYPTO_ALLOW_LEGACY_NO_AAD", "false")
	security.ResetFieldCryptoForTests()
	t.Cleanup(security.ResetFieldCryptoForTests)
	conn := sql.OpenDB(metadataFixtureConnector{})
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTaskLogsAndOwnershipReadWithoutDecryptingCredentials(t *testing.T) {
	db := newMetadataFixtureDB(t)
	accounts := NewAccountRepository(db)
	if _, err := accounts.FindByID(1); !errors.Is(err, security.ErrCredentialUnreadable) {
		t.Fatalf("execution query must still reject unreadable credentials: %v", err)
	}
	account, err := accounts.FindMetadataByID(1)
	if err != nil || account.Phone != "fixture-phone" || account.Auth != "" {
		t.Fatalf("metadata account=%+v err=%v", account, err)
	}
	logs, count, err := NewTaskLogRepository(db).FindByFilter(7, nil, "", "", 0, 20)
	if err != nil || count != 1 || len(logs) != 1 || logs[0].Account.Phone != "fixture-phone" || logs[0].Account.Auth != "" {
		t.Fatalf("task log display failed on credential errors: logs=%+v count=%d err=%v", logs, count, err)
	}
}

func TestExchangeListsPreserveRelationsWithoutDecryptingCredentials(t *testing.T) {
	db := newMetadataFixtureDB(t)
	for _, admin := range []bool{false, true} {
		rules := NewExchangeAccountRepository(db)
		list, err := rules.GetByUserID(7)
		if admin {
			list, err = rules.GetAll()
		}
		if err != nil || len(list) != 1 || list[0].Auth != "" || list[0].Account.Auth != "" || list[0].Account.Phone != "fixture-phone" {
			t.Fatalf("rule list admin=%t err=%v list=%+v", admin, err, list)
		}
		if len(list[0].Tasks) != 1 || list[0].Tasks[0].Product.PrizeName != "fixture-product" {
			t.Fatal("rule display lost current product information")
		}
		tasks := NewExchangeTaskRepository(db)
		items, err := tasks.GetByUserIDWithFilter(7, ExchangeTaskFilter{})
		if admin {
			items, err = tasks.GetAllWithFilter(ExchangeTaskFilter{})
		}
		if err != nil || len(items) != 1 || items[0].ExchangeAccount.Auth != "" || items[0].ExchangeAccount.Account.Auth != "" || items[0].ExchangeAccount.Account.Phone != "fixture-phone" {
			t.Fatalf("task list admin=%t err=%v items=%+v", admin, err, items)
		}
	}
}
