package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This fixture executes the real GORM UPDATE against a single in-memory SQL
// row. Its generic SET/WHERE evaluator verifies fencing predicates without
// requiring CGO; deployed MySQL concurrency remains an integration gate.
type exchangeClaimSQLFixture struct {
	mu  sync.Mutex
	row map[string]driver.Value
}
type exchangeClaimConnector struct{ fixture *exchangeClaimSQLFixture }
type exchangeClaimDriver struct{ fixture *exchangeClaimSQLFixture }
type exchangeClaimConn struct {
	metadataFixtureConn
	fixture *exchangeClaimSQLFixture
}

func (c exchangeClaimConnector) Connect(context.Context) (driver.Conn, error) {
	return exchangeClaimConn{fixture: c.fixture}, nil
}
func (c exchangeClaimConnector) Driver() driver.Driver { return exchangeClaimDriver{c.fixture} }
func (d exchangeClaimDriver) Open(string) (driver.Conn, error) {
	return exchangeClaimConn{fixture: d.fixture}, nil
}

var exchangeClaimSQLPredicate = regexp.MustCompile("`?([a-z_]+)`?\\s*=\\s*\\?")

func (c exchangeClaimConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.fixture.mu.Lock()
	defer c.fixture.mu.Unlock()
	setAt, whereAt := strings.Index(query, " SET "), strings.Index(query, " WHERE ")
	if setAt < 0 || whereAt < 0 || !strings.HasPrefix(query, "UPDATE `exchange_tasks`") {
		return nil, fmt.Errorf("unexpected fixture write: %s", query)
	}
	setFields := exchangeClaimSQLPredicate.FindAllStringSubmatch(query[setAt:whereAt], -1)
	guards := exchangeClaimSQLPredicate.FindAllStringSubmatch(query[whereAt:], -1)
	if len(setFields)+len(guards) != len(args) {
		return nil, fmt.Errorf("unparsed fixture bindings: %s", query)
	}
	if c.fixture.row["deleted_at"] != nil {
		return driver.RowsAffected(0), nil
	}
	for i, guard := range guards {
		stored, requested := c.fixture.row[guard[1]], args[len(setFields)+i].Value
		matches := reflect.DeepEqual(stored, requested)
		if at, ok := stored.(time.Time); ok {
			other, valid := requested.(time.Time)
			matches = valid && at.Equal(other)
		}
		if !matches {
			return driver.RowsAffected(0), nil
		}
	}
	for i, field := range setFields {
		c.fixture.row[field[1]] = args[i].Value
	}
	return driver.RowsAffected(1), nil
}

func newExchangeClaimSQLRepository(t *testing.T, status string) (*ExchangeTaskRepository, *exchangeClaimSQLFixture, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 1, 10, 8, 41, 0, time.UTC)
	fixture := &exchangeClaimSQLFixture{row: map[string]driver.Value{
		"id": int64(156), "user_id": int64(7), "status": status, "updated_at": now,
		"retry_count": int64(3), "last_result": "历史 GK", "attempted_count": int64(4), "fail_count": int64(4),
	}}
	conn := sql.OpenDB(exchangeClaimConnector{fixture})
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, SkipDefaultTransaction: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewExchangeTaskRepository(db), fixture, now
}

func TestManualClaimSQLRestartsFailedTaskWithoutErasingAttemptHistory(t *testing.T) {
	repo, fixture, now := newExchangeClaimSQLRepository(t, "failed")
	claimed, token, err := repo.TryMarkForManualExecution(156, 7, "failed", now)
	if err != nil || !claimed || token == "" || fixture.row["status"] != "running" || fixture.row["execution_token"] != token || fixture.row["retry_count"] != int64(0) || fixture.row["last_result"] != "" {
		t.Fatalf("failed task was not atomically restarted: claimed=%t token=%q err=%v row=%v", claimed, token, err, fixture.row)
	}
	if fixture.row["attempted_count"] != int64(4) || fixture.row["fail_count"] != int64(4) {
		t.Fatal("historical attempt counters were reset")
	}
}

func TestManualClaimSQLFencesOwnerSnapshotAndTerminalStates(t *testing.T) {
	for _, tc := range []struct {
		status string
		userID uint
		stale  bool
	}{
		{"failed", 8, false}, {"failed", 7, true},
		{"running", 7, false}, {"completed", 7, false},
	} {
		repo, fixture, at := newExchangeClaimSQLRepository(t, tc.status)
		if tc.stale {
			at = at.Add(-time.Second)
		}
		claimed, _, err := repo.TryMarkForManualExecution(156, tc.userID, "failed", at)
		if err != nil || claimed || fixture.row["status"] != tc.status || fixture.row["retry_count"] != int64(3) {
			t.Fatalf("fencing failed for %+v: claimed=%t err=%v row=%v", tc, claimed, err, fixture.row)
		}
	}
}

func TestManualClaimSQLConcurrentRetriesAcquireOneFencingToken(t *testing.T) {
	repo, _, at := newExchangeClaimSQLRepository(t, "failed")
	var wg sync.WaitGroup
	results := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			claimed, _, err := repo.TryMarkForManualExecution(156, 7, "failed", at)
			if err != nil {
				t.Errorf("claim error: %v", err)
			}
			results <- claimed
		}()
	}
	wg.Wait()
	close(results)
	count := 0
	for claimed := range results {
		if claimed {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("concurrent manual retries claimed %d tokens", count)
	}
}
