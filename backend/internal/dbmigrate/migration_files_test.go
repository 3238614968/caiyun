package dbmigrate

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

var expectedMigrationFiles = []string{
	"001_init.sql",
	"002_exchange_task_schedule.sql",
	"003_exchange_rule_alias.sql",
	"004_exchange_schedule_strategy.sql",
	"005_calendar_dates.sql",
	"006_exchange_rule_physical_rename.sql",
	"007_log_archive_and_indexes.sql",
	"008_exchange_task_idempotency.sql",
	"009_audit_request_id.sql",
	"010_operations.sql",
	"011_refresh_sessions.sql",
	"012_normalized_user_identity.sql",
	"013_websocket_delivery.sql",
	"014_exchange_task_operation_id.sql",
	"015_exchange_task_rule_fk.sql",
	"016_exchange_task_schedule_dedupe.sql",
	"017_exchange_record_rule_fk.sql",
	"018_execution_fencing_tokens.sql",
	"019_websocket_sequence_allocator.sql",
	"020_cloud_stats_account_date_unique.sql",
	"021_archive_schema_parity_and_exchange_rule_unique.sql",
	"022_audit_log_anonymous_actor.sql",
}

func TestMigrationSQLDoesNotWriteSchemaVersionAndCopiesMatch(t *testing.T) {
	embeddedNames := embeddedMigrationFileNames(t)
	if !reflect.DeepEqual(embeddedNames, expectedMigrationFiles) {
		t.Fatalf("embedded migration files = %#v, want contiguous 001-022 %#v", embeddedNames, expectedMigrationFiles)
	}

	externalDir := filepath.Join("..", "..", "migrations")
	externalNames := externalMigrationFileNames(t, externalDir)
	if !reflect.DeepEqual(externalNames, expectedMigrationFiles) {
		t.Fatalf("external migration files = %#v, want contiguous 001-022 %#v", externalNames, expectedMigrationFiles)
	}

	for _, name := range expectedMigrationFiles {
		embedded, err := migrationFS.ReadFile("sql/" + name)
		if err != nil {
			t.Fatalf("read embedded migration %s: %v", name, err)
		}
		if strings.Contains(strings.ToLower(string(embedded)), "schema_migrations") {
			t.Fatalf("migration %s must not create or record schema_migrations; the runner owns version tracking", name)
		}

		externalPath := filepath.Join(externalDir, name)
		external, err := os.ReadFile(externalPath)
		if err != nil {
			t.Fatalf("read migration copy %s: %v", externalPath, err)
		}
		if !bytes.Equal(embedded, external) {
			t.Fatalf("embedded migration and external copy differ: %s", name)
		}
	}

	baseline, err := migrationFS.ReadFile("sql/001_init.sql")
	if err != nil {
		t.Fatalf("read embedded baseline: %v", err)
	}
	if _, err := os.Stat(filepath.Join(externalDir, "init.sql")); !os.IsNotExist(err) {
		t.Fatal("non-versioned migrations/init.sql must not exist; use 001_init.sql")
	}
	for _, incrementalOnly := range []string{
		"CREATE TABLE IF NOT EXISTS `operations`",
		"CREATE TABLE IF NOT EXISTS `refresh_sessions`",
		"`normalized_username`",
		"'message_id'",
		"'acked_at'",
		"'source_operation_id'",
	} {
		if bytes.Contains(baseline, []byte(incrementalOnly)) {
			t.Fatalf("001_init.sql must remain a baseline and must not merge incremental token %q", incrementalOnly)
		}
	}
}

func TestExchangeTaskDedupeUsesVirtualGeneratedColumn(t *testing.T) {
	content, err := migrationFS.ReadFile("sql/008_exchange_task_idempotency.sql")
	if err != nil {
		t.Fatalf("read exchange task idempotency migration: %v", err)
	}
	upper := bytes.ToUpper(content)
	if !bytes.Contains(upper, []byte("GENERATED ALWAYS AS")) || !bytes.Contains(upper, []byte(" VIRTUAL'")) {
		t.Fatal("008 migration must use an indexable VIRTUAL generated column")
	}
	if bytes.Contains(upper, []byte(" STORED'")) {
		t.Fatal("008 migration must not use a STORED generated column because its base columns have cascading foreign keys")
	}
}

func TestRestartSensitiveMigrationsUseIdempotentGuards(t *testing.T) {
	dedupe, err := migrationFS.ReadFile("sql/016_exchange_task_schedule_dedupe.sql")
	if err != nil {
		t.Fatalf("read 016 migration: %v", err)
	}
	for _, token := range []string{"AddColumnIfMissing", "CreateUniqueIndexIfMissing", "DROP PROCEDURE IF EXISTS `AddColumnIfMissing`"} {
		if !bytes.Contains(dedupe, []byte(token)) {
			t.Fatalf("016 migration must protect interrupted re-runs with %q", token)
		}
	}

	fencing, err := migrationFS.ReadFile("sql/018_execution_fencing_tokens.sql")
	if err != nil {
		t.Fatalf("read 018 migration: %v", err)
	}
	if !bytes.Contains(fencing, []byte("DROP PROCEDURE IF EXISTS `AddColumnIfMissing`$$")) {
		t.Fatal("018 migration must clear a helper procedure left by an interrupted run")
	}
}

func TestCriticalMigrationColumnsPresent(t *testing.T) {
	required := map[string][]string{
		"010_operations.sql": {
			"CREATE TABLE IF NOT EXISTS `operations`", "`id`", "`user_id`", "`operation_type`", "`status`",
			"`account_id`", "`resource_id`", "`payload`", "`idempotency_key`", "`attempt_count`",
			"`error_summary`", "`queued_at`", "`started_at`", "`completed_at`", "`created_at`", "`updated_at`",
		},
		"011_refresh_sessions.sql": {
			"CREATE TABLE IF NOT EXISTS `refresh_sessions`", "`id`", "`user_id`", "`refresh_token_hash`",
			"`token_version`", "`device_info`", "`expires_at`", "`revoked_at`", "`replaced_by_session_id`",
			"`last_used_at`", "`created_at`", "`updated_at`",
		},
		"012_normalized_user_identity.sql": {
			"'normalized_username'", "'normalized_email'", "uk_users_normalized_username", "uk_users_normalized_email",
		},
		"013_websocket_delivery.sql": {
			"'web_socket_messages'", "'message_id'", "'sequence'", "'expires_at'", "'acked_at'",
			"uidx_ws_message_id", "idx_ws_user_sequence", "idx_ws_expires_at",
		},
		"014_exchange_task_operation_id.sql": {
			"'exchange_tasks'", "'source_operation_id'", "uk_exchange_tasks_source_operation",
		},
		"016_exchange_task_schedule_dedupe.sql": {
			"active_dedupe_key", "scheduled_exchange_time", "uk_exchange_tasks_active_dedupe",
		},
		"017_exchange_record_rule_fk.sql": {
			"exchange_records", "exchange_account_id", "MODIFY COLUMN",
		},
		"018_execution_fencing_tokens.sql": {
			"operations", "exchange_tasks", "execution_token", "AddColumnIfMissing",
		},
		"019_websocket_sequence_allocator.sql": {
			"CREATE TABLE IF NOT EXISTS `web_socket_sequences`", "PRIMARY KEY (`user_id`)",
			"INSERT INTO `web_socket_sequences`", "MAX(`sequence`)", "ON DUPLICATE KEY UPDATE",
		},
		"020_cloud_stats_account_date_unique.sql": {
			"DELETE older", "`deleted_at` IS NOT NULL", "`deleted_at` IS NULL", "cloud_stats", "uk_cloud_stats_account_date", "CreateUniqueIndexIfMissing",
		},
		"021_archive_schema_parity_and_exchange_rule_unique.sql": {
			"exchange_records_archive", "exchange_rule_id", "MakeColumnNullableIfRequired",
			"active_account_id", "uk_exchange_rules_active_account", "idx_users_role_id",
		},
		"022_audit_log_anonymous_actor.sql": {
			"audit_logs", "DropAuditLogUserForeignKey", "MakeAuditLogUserIDNullable", "MODIFY COLUMN `user_id` BIGINT UNSIGNED NULL",
		},
	}

	for name, tokens := range required {
		content, err := migrationFS.ReadFile("sql/" + name)
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		for _, token := range tokens {
			if !bytes.Contains(content, []byte(token)) {
				t.Errorf("migration %s missing critical schema token %q", name, token)
			}
		}
	}
}

func embeddedMigrationFileNames(t *testing.T) []string {
	t.Helper()
	entries, err := migrationFS.ReadDir("sql")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func externalMigrationFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read external migrations: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || len(name) < 5 || name[3] != '_' || !strings.HasSuffix(name, ".sql") {
			continue
		}
		if name[0] < '0' || name[0] > '9' || name[1] < '0' || name[1] > '9' || name[2] < '0' || name[2] > '9' {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
