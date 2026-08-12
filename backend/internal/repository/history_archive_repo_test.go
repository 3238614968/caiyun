package repository

import (
	"strings"
	"testing"
)

func TestArchiveInsertSQLUsesExplicitSchemaAlignedColumns(t *testing.T) {
	tests := []struct {
		source  string
		archive string
		must    []string
	}{
		{
			source:  "task_logs",
			archive: "task_logs_archive",
			must:    []string{"`task_type`", "`deleted_at`"},
		},
		{
			source:  "exchange_records",
			archive: "exchange_records_archive",
			must:    []string{"`exchange_account_id`", "`exchange_rule_id`", "`exchange_task_id`"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			spec, err := archiveTableSpecFor(tt.source, tt.archive)
			if err != nil {
				t.Fatalf("archiveTableSpecFor: %v", err)
			}
			sql := buildArchiveInsertSQL(tt.source, tt.archive, spec.columns, "?, ?")
			if strings.Contains(sql, "SELECT *") {
				t.Fatalf("archive SQL must not use SELECT *: %s", sql)
			}
			if !strings.Contains(sql, "INSERT INTO `"+tt.archive+"` (") || !strings.Contains(sql, "FROM `"+tt.source+"`") {
				t.Fatalf("archive SQL targets the wrong tables: %s", sql)
			}
			for _, column := range tt.must {
				if strings.Count(sql, column) != 2 {
					t.Fatalf("archive SQL must explicitly copy %s in source and target lists: %s", column, sql)
				}
			}
		})
	}
}

func TestArchiveTableSpecRejectsUnapprovedPair(t *testing.T) {
	if _, err := archiveTableSpecFor("task_logs", "exchange_records_archive"); err == nil {
		t.Fatal("mismatched archive table pair must be rejected")
	}
}
