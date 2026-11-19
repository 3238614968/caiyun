package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The driver executes the repository's actual GORM transaction and observes
// the emitted locking query. No CGO/database service is needed for this guard
// against accidentally reverting to an unlocked snapshot of the source rows.
type archiveTransactionFixture struct {
	inTransaction bool
	sourceLocked  bool
	copied        bool
	committed     bool
	rolledBack    bool
	deleteRows    int64
}

type archiveTransactionConnector struct{ fixture *archiveTransactionFixture }
type archiveTransactionDriver struct{}
type archiveTransactionConn struct{ fixture *archiveTransactionFixture }
type archiveTransactionTx struct{ fixture *archiveTransactionFixture }
type archiveTransactionRows struct{ returned bool }

func (c archiveTransactionConnector) Connect(context.Context) (driver.Conn, error) {
	return &archiveTransactionConn{fixture: c.fixture}, nil
}
func (archiveTransactionConnector) Driver() driver.Driver { return archiveTransactionDriver{} }
func (archiveTransactionDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use archive fixture connector")
}
func (*archiveTransactionConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("archive fixture requires contextual operations")
}
func (*archiveTransactionConn) Close() error { return nil }
func (c *archiveTransactionConn) Begin() (driver.Tx, error) {
	c.fixture.inTransaction = true
	return &archiveTransactionTx{fixture: c.fixture}, nil
}
func (tx *archiveTransactionTx) Commit() error {
	tx.fixture.committed = true
	tx.fixture.inTransaction = false
	return nil
}
func (tx *archiveTransactionTx) Rollback() error {
	tx.fixture.rolledBack = true
	tx.fixture.inTransaction = false
	tx.fixture.copied = false
	return nil
}
func (*archiveTransactionRows) Columns() []string { return []string{"id"} }
func (*archiveTransactionRows) Close() error      { return nil }
func (r *archiveTransactionRows) Next(values []driver.Value) error {
	if r.returned {
		return io.EOF
	}
	r.returned = true
	values[0] = int64(42)
	return nil
}
func (c *archiveTransactionConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !c.fixture.inTransaction || !strings.HasPrefix(query, "SELECT id FROM ") || !strings.HasSuffix(query, " FOR UPDATE") {
		return nil, fmt.Errorf("archiver must lock selected live rows in its transaction: %s", query)
	}
	c.fixture.sourceLocked = true
	return &archiveTransactionRows{}, nil
}
func (c *archiveTransactionConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if !c.fixture.inTransaction || !c.fixture.sourceLocked {
		return nil, errors.New("copy/deletion attempted without a transaction-owned source lock")
	}
	if strings.HasPrefix(query, "INSERT INTO ") {
		c.fixture.copied = true
		return driver.RowsAffected(1), nil
	}
	if strings.HasPrefix(query, "DELETE FROM ") && c.fixture.copied {
		return driver.RowsAffected(c.fixture.deleteRows), nil
	}
	return nil, fmt.Errorf("unexpected archive query: %s", query)
}

func newArchiveTransactionRepository(t *testing.T, deleteRows int64) (*HistoryArchiveRepository, *archiveTransactionFixture) {
	t.Helper()
	fixture := &archiveTransactionFixture{deleteRows: deleteRows}
	conn := sql.OpenDB(archiveTransactionConnector{fixture: fixture})
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: conn, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewHistoryArchiveRepository(db), fixture
}

func TestHistoryArchiveLocksSourceRowsUntilCopyAndDeletionCommit(t *testing.T) {
	for _, source := range []string{"task_logs", "exchange_records"} {
		t.Run(source, func(t *testing.T) {
			repo, fixture := newArchiveTransactionRepository(t, 1)
			moved, err := repo.archiveBatch(source, source+"_archive", time.Now(), 100)
			if err != nil || moved != 1 || !fixture.sourceLocked || !fixture.committed || fixture.rolledBack {
				t.Fatalf("archive moved=%d error=%v fixture=%+v", moved, err, fixture)
			}
		})
	}
}

func TestHistoryArchiveRollsBackCopyIfSourceDeletionIsIncomplete(t *testing.T) {
	repo, fixture := newArchiveTransactionRepository(t, 0)
	moved, err := repo.ArchiveTaskLogsBefore(t.Context(), time.Now(), 100)
	if err == nil || moved != 0 || !fixture.rolledBack || fixture.committed || fixture.copied {
		t.Fatalf("incomplete deletion committed archive data: moved=%d error=%v fixture=%+v", moved, err, fixture)
	}
}
