package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Exercise the real repository and GORM transaction path on CGO-free hosts.
// The fixture retains rows by owner and commits/rolls back state, rather than
// merely comparing an expected list of SQL calls.
type userCleanupState struct {
	rows      map[string][]int64
	failTable string
}

type userCleanupConnector struct{ state *userCleanupState }
type userCleanupDriver struct{}
type userCleanupConn struct {
	state   *userCleanupState
	pending map[string][]int64
}
type userCleanupTx struct{ conn *userCleanupConn }

func (c userCleanupConnector) Connect(context.Context) (driver.Conn, error) {
	return &userCleanupConn{state: c.state}, nil
}
func (userCleanupConnector) Driver() driver.Driver { return userCleanupDriver{} }
func (userCleanupDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}
func (*userCleanupConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("fixture requires ExecContext")
}
func (*userCleanupConn) Close() error { return nil }
func (c *userCleanupConn) Begin() (driver.Tx, error) {
	if c.pending != nil {
		return nil, errors.New("transaction already open")
	}
	c.pending = make(map[string][]int64, len(c.state.rows))
	for table, rows := range c.state.rows {
		c.pending[table] = append([]int64(nil), rows...)
	}
	return &userCleanupTx{conn: c}, nil
}
func (tx *userCleanupTx) Commit() error {
	tx.conn.state.rows = tx.conn.pending
	tx.conn.pending = nil
	return nil
}
func (tx *userCleanupTx) Rollback() error {
	tx.conn.pending = nil
	return nil
}

var userCleanupDeletePattern = regexp.MustCompile("^DELETE FROM `([a-z_]+)` WHERE user_id = \\?$")

func (c *userCleanupConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	match := userCleanupDeletePattern.FindStringSubmatch(query)
	if len(match) != 2 || len(args) != 1 || c.pending == nil {
		return nil, fmt.Errorf("unexpected deletion outside user transaction: %s", query)
	}
	table := match[1]
	if table == c.state.failTable {
		return nil, errors.New("fixture archive/storage failure")
	}
	rows, ok := c.pending[table]
	if !ok {
		return nil, fmt.Errorf("unexpected table %s", table)
	}
	owner, ok := args[0].Value.(int64)
	if !ok {
		return nil, fmt.Errorf("unexpected owner %T", args[0].Value)
	}
	var kept []int64
	var deleted int64
	for _, userID := range rows {
		if userID == owner {
			deleted++
		} else {
			kept = append(kept, userID)
		}
	}
	c.pending[table] = kept
	return driver.RowsAffected(deleted), nil
}

func newUserCleanupDB(t *testing.T, failTable string) (*gorm.DB, *userCleanupState) {
	t.Helper()
	state := &userCleanupState{rows: map[string][]int64{}, failTable: failTable}
	for _, table := range []string{"exchange_records", "exchange_records_archive", "task_logs", "task_logs_archive"} {
		state.rows[table] = []int64{7, 7, 8}
	}
	conn := sql.OpenDB(userCleanupConnector{state: state})
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, state
}

func TestUserErasureRemovesLiveAndArchivedHistoryForOnlyTargetUser(t *testing.T) {
	db, state := newUserCleanupDB(t, "")
	err := NewUnitOfWork(db).WithinTransaction(t.Context(), func(repos TransactionRepositories) error {
		if err := repos.ExchangeRecord.DeleteByUserID(7); err != nil {
			return err
		}
		return repos.TaskLog.DeleteByUserID(7)
	})
	if err != nil {
		t.Fatal(err)
	}
	for table, rows := range state.rows {
		if len(rows) != 1 || rows[0] != 8 {
			t.Errorf("%s retained owners = %v, want only unrelated user 8", table, rows)
		}
	}
}

func TestUserErasureRollsBackLiveAndArchivedHistoryOnFailure(t *testing.T) {
	for _, failure := range []string{"task_logs_archive", "task_logs", "later user cleanup"} {
		t.Run(failure, func(t *testing.T) {
			db, state := newUserCleanupDB(t, failure)
			err := NewUnitOfWork(db).WithinTransaction(t.Context(), func(repos TransactionRepositories) error {
				if err := repos.ExchangeRecord.DeleteByUserID(7); err != nil {
					return err
				}
				if err := repos.TaskLog.DeleteByUserID(7); err != nil {
					return err
				}
				return errors.New("later user cleanup failure")
			})
			if err == nil {
				t.Fatal("expected storage/cleanup failure")
			}
			for table, rows := range state.rows {
				if fmt.Sprint(rows) != "[7 7 8]" {
					t.Errorf("%s retained owners after rollback = %v", table, rows)
				}
			}
			if !strings.Contains(err.Error(), "failure") {
				t.Fatalf("unexpected failure: %v", err)
			}
		})
	}
}
