package repository

import (
	"caiyun/internal/models"
	"caiyun/internal/security"
	"context"
	"database/sql/driver"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

type accountLoginSQLFixture struct{ updates map[string]driver.Value }
type accountLoginConnector struct{ fixture *accountLoginSQLFixture }
type accountLoginDriver struct{ fixture *accountLoginSQLFixture }
type accountLoginConn struct {
	metadataFixtureConn
	fixture *accountLoginSQLFixture
}

func (c accountLoginConnector) Connect(context.Context) (driver.Conn, error) {
	return accountLoginConn{fixture: c.fixture}, nil
}
func (c accountLoginConnector) Driver() driver.Driver { return accountLoginDriver{c.fixture} }
func (c accountLoginDriver) Open(string) (driver.Conn, error) {
	return accountLoginConn{fixture: c.fixture}, nil
}
func (c accountLoginConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.metadataFixtureConn.QueryContext(ctx, q, args)
	if err != nil || !strings.Contains(q, "FROM `accounts`") {
		return rows, err
	}
	r := rows.(*metadataFixtureRows)
	for i, column := range r.columns {
		switch column {
		case "deleted_at":
			r.values[i] = time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
		case "cloud_count":
			r.values[i] = int64(1234)
		case "created_at":
			r.values[i] = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		}
	}
	return r, nil
}

var accountLoginSetColumn = regexp.MustCompile("`([a-z_]+)`\\s*=\\s*\\?")

func (c accountLoginConn) ExecContext(_ context.Context, q string, args []driver.NamedValue) (driver.Result, error) {
	where := strings.Index(q, " WHERE ")
	if !strings.HasPrefix(q, "UPDATE `accounts`") || where < 0 {
		return nil, fmt.Errorf("unexpected account write")
	}
	fields := accountLoginSetColumn.FindAllStringSubmatch(q[:where], -1)
	c.fixture.updates = make(map[string]driver.Value)
	for i, field := range fields {
		c.fixture.updates[field[1]] = args[i].Value
	}
	return driver.RowsAffected(1), nil
}

func TestAccountLoginRestoresMetadataAndReplacesUnreadableCredentials(t *testing.T) {
	fixture := &accountLoginSQLFixture{}
	db := newMetadataFixtureDBWithConnector(t, accountLoginConnector{fixture})
	db = db.Session(&gorm.Session{SkipDefaultTransaction: true})
	repo := NewAccountRepository(db)
	account := &models.Account{UserID: 7, Phone: "fixture-phone", Auth: "new-auth", Token: "new-token", Platform: "android"}
	if err := repo.SaveLogin(account); err != nil {
		t.Fatal(err)
	}
	if account.ID != 1 || account.CloudCount != 1234 || account.DeletedAt.Valid || !account.IsActive {
		t.Fatalf("history not restored: %+v", account)
	}
	if fixture.updates["deleted_at"] != nil || fixture.updates["is_active"] != true || fixture.updates["jwt_token"] != "" {
		t.Fatalf("restored state invalid: %v", fixture.updates)
	}
	for field, want := range map[string]string{"auth": "new-auth", "token": "new-token"} {
		value := fixture.updates[field].(string)
		decoded, err := security.DecryptString(value)
		if err != nil || decoded != want || !security.IsEncryptedValue(value) {
			t.Fatalf("new credential not protected: field=%s err=%v", field, err)
		}
	}
	if _, erased := fixture.updates["cloud_count"]; erased {
		t.Fatal("login overwrote cloud history")
	}
}

func TestAccountRemovalOnlyClearsAccessAndSetsTombstone(t *testing.T) {
	fixture := &accountLoginSQLFixture{}
	db := newMetadataFixtureDBWithConnector(t, accountLoginConnector{fixture}).Session(&gorm.Session{SkipDefaultTransaction: true})
	if err := NewAccountRepository(db).Delete(1); err != nil {
		t.Fatal(err)
	}
	if fixture.updates["deleted_at"] == nil || fixture.updates["is_active"] != false || fixture.updates["auth"] != "" || fixture.updates["token"] != "" || fixture.updates["jwt_token"] != "" {
		t.Fatalf("account remained usable: %v", fixture.updates)
	}
	if _, changed := fixture.updates["phone"]; changed {
		t.Fatal("removed account identity was erased")
	}
}
