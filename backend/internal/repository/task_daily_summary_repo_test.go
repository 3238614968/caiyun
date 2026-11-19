package repository

import (
	"context"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type dailySummaryFixture struct {
	metadataFixtureConn
	t     *testing.T
	start time.Time
	calls int
}

type dailySummaryConnector struct{ fixture *dailySummaryFixture }

func (c dailySummaryConnector) Connect(context.Context) (driver.Conn, error) { return c.fixture, nil }
func (c dailySummaryConnector) Driver() driver.Driver                        { return metadataFixtureDriver{} }

type dailySummaryRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *dailySummaryRows) Columns() []string { return r.columns }
func (r *dailySummaryRows) Close() error      { return nil }
func (r *dailySummaryRows) Next(values []driver.Value) error {
	if r.index == len(r.values) {
		return io.EOF
	}
	copy(values, r.values[r.index])
	r.index++
	return nil
}

func (f *dailySummaryFixture) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.calls++
	sql := strings.ToLower(strings.ReplaceAll(query, "`", ""))
	if !strings.Contains(sql, "user_id = ?") || !strings.Contains(sql, "deleted_at") {
		return nil, fmt.Errorf("summary lost user or soft-delete scope: %s", query)
	}
	if strings.Contains(sql, "from accounts") {
		if strings.Contains(sql, "auth") || strings.Contains(sql, "token") {
			return nil, fmt.Errorf("summary fetched credentials")
		}
		return &dailySummaryRows{columns: []string{"id", "phone", "remark"}, values: [][]driver.Value{{int64(2), "13900000001", "测试账号"}}}, nil
	}
	if !strings.Contains(sql, "created_at >= ? and created_at < ?") || !strings.Contains(sql, "task_type not in") {
		return nil, fmt.Errorf("summary lost day boundary or snapshot exclusion: %s", query)
	}
	var user, start, end bool
	for _, arg := range args {
		switch value := arg.Value.(type) {
		case int64:
			user = user || value == 7
		case time.Time:
			start = start || value.Equal(f.start)
			end = end || value.Equal(f.start.AddDate(0, 0, 1))
		}
	}
	if !user || !start || !end {
		return nil, fmt.Errorf("summary scope arguments missing: %v", args)
	}
	if strings.Contains(sql, "count(") {
		if !strings.Contains(sql, "distinct") {
			return nil, fmt.Errorf("pagination counts log rows, not accounts")
		}
		return &dailySummaryRows{columns: []string{"count"}, values: [][]driver.Value{{int64(1)}}}, nil
	}
	if strings.Contains(sql, "as cloud_gained") {
		if !strings.Contains(sql, "sum(") || !strings.Contains(sql, "group by") || !strings.Contains(sql, "limit") {
			return nil, fmt.Errorf("unbounded or ungrouped summary: %s", query)
		}
		if !strings.Contains(sql, "sum(case when migu_notes_gained > 0") {
			return nil, fmt.Errorf("summary lost separate note receipts: %s", query)
		}
		return &dailySummaryRows{columns: []string{"account_id", "cloud_gained", "migu_notes_gained"}, values: [][]driver.Value{{int64(2), int64(11), int64(40)}}}, nil
	}
	if !strings.Contains(sql, "max(id)") || !strings.Contains(sql, "group by account_id, task_type") {
		return nil, fmt.Errorf("retries are counted repeatedly: %s", query)
	}
	return &dailySummaryRows{columns: []string{"account_id", "status", "created_at"}, values: [][]driver.Value{{int64(2), "success", f.start.Add(time.Hour)}, {int64(2), "failed", f.start.Add(30 * time.Minute)}, {int64(2), "running", f.start.Add(15 * time.Minute)}}}, nil
}

func TestDailySummaryScopesWholeDayAndCountsLatestTaskOutcomes(t *testing.T) {
	fixture := &dailySummaryFixture{t: t, start: time.Date(2026, 11, 12, 0, 0, 0, 0, cstZone)}
	repo := NewTaskLogRepository(newMetadataFixtureDBWithConnector(t, dailySummaryConnector{fixture}))
	rows, total, err := repo.DailySummaries(context.Background(), 7, fixture.start, fixture.start.AddDate(0, 0, 1), 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(rows) != 1 || fixture.calls != 4 {
		t.Fatalf("wrong grouped pagination: total=%d rows=%d calls=%d", total, len(rows), fixture.calls)
	}
	row := rows[0]
	if row.AccountID != 2 || row.CloudGained != 11 || row.MiguNotesGained != 40 || row.SuccessCount != 1 || row.FailedCount != 1 || row.PendingCount != 1 || row.Phone != "13900000001" {
		t.Fatalf("summary lost receipts or misclassified ended attempts: %+v", row)
	}
}

type latestOutcomeFixture struct {
	metadataFixtureConn
	queries int
}
type latestOutcomeConnector struct{ fixture *latestOutcomeFixture }

func (c latestOutcomeConnector) Connect(context.Context) (driver.Conn, error) { return c.fixture, nil }
func (c latestOutcomeConnector) Driver() driver.Driver                        { return metadataFixtureDriver{} }
func (f *latestOutcomeFixture) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	f.queries++
	sql := strings.ToLower(strings.ReplaceAll(query, "`", ""))
	latest := strings.Index(sql, "max(id)")
	if latest < 0 || strings.Index(sql, "status = ?") < latest || !strings.Contains(sql, "group by account_id, task_type") || !strings.Contains(sql, "created_at >= ? and created_at < ?") || !strings.Contains(sql, "user_id = ?") {
		return nil, fmt.Errorf("failure filter must follow the scoped latest-result selection: %s", query)
	}
	var user, account, failed bool
	for _, arg := range args {
		user = user || arg.Value == int64(7)
		account = account || arg.Value == int64(2)
		failed = failed || arg.Value == "failed"
	}
	if !user || !account || !failed {
		return nil, fmt.Errorf("missing drill-down scope")
	}
	if strings.Contains(sql, "count(") {
		return &dailySummaryRows{columns: []string{"count"}, values: [][]driver.Value{{int64(0)}}}, nil
	}
	return &dailySummaryRows{columns: []string{"id", "account_id", "status"}}, nil
}

func TestDailyFailureDrillDownAppliesStatusAfterLatestAttemptSelection(t *testing.T) {
	fixture := &latestOutcomeFixture{}
	repo := NewTaskLogRepository(newMetadataFixtureDBWithConnector(t, latestOutcomeConnector{fixture}))
	start := time.Date(2026, 11, 12, 0, 0, 0, 0, cstZone)
	accountID := uint(2)
	rows, total, err := repo.FindByFilterInRange(context.Background(), 7, &accountID, "", "failed", false, start, start.AddDate(0, 0, 1), true, 0, 20)
	if err != nil || total != 0 || len(rows) != 0 || fixture.queries != 2 {
		t.Fatalf("latest-result drill-down failed: total=%d rows=%d queries=%d err=%v", total, len(rows), fixture.queries, err)
	}
}
