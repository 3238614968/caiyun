package dbmigrate

import (
	"strings"
	"testing"
)

func TestBaselineRequestIDIndexesReferenceExistingColumns(t *testing.T) {
	content, err := migrationFS.ReadFile("sql/001_init.sql")
	if err != nil {
		t.Fatalf("read baseline migration: %v", err)
	}
	sql := string(content)

	for _, table := range []string{"task_logs", "exchange_records", "web_socket_messages"} {
		block := migrationTableBlock(t, sql, table)
		if strings.Contains(block, "idx_request_id") {
			t.Fatalf("table %s creates idx_request_id without defining request_id", table)
		}
	}

	auditBlock := migrationTableBlock(t, sql, "audit_logs")
	if !strings.Contains(auditBlock, "`request_id` VARCHAR(128)") || !strings.Contains(auditBlock, "idx_request_id") {
		t.Fatal("audit_logs must define request_id before creating idx_request_id")
	}
}

func migrationTableBlock(t *testing.T, sql, table string) string {
	t.Helper()
	marker := "CREATE TABLE `" + table + "`"
	start := strings.Index(sql, marker)
	if start < 0 {
		marker = "CREATE TABLE IF NOT EXISTS `" + table + "`"
		start = strings.Index(sql, marker)
	}
	if start < 0 {
		t.Fatalf("table %s not found in baseline migration", table)
	}
	rest := sql[start:]
	end := strings.Index(rest, ") ENGINE=InnoDB")
	if end < 0 {
		t.Fatalf("table %s block terminator not found", table)
	}
	return rest[:end]
}
