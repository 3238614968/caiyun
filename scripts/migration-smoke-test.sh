#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MIGRATIONS_DIR="$ROOT/backend/migrations"
EMBEDDED_MIGRATIONS_DIR="$ROOT/backend/internal/dbmigrate/sql"

mapfile -t versioned_migrations < <(cd "$MIGRATIONS_DIR" && find . -maxdepth 1 -type f -name '[0-9][0-9][0-9]_*.sql' -printf '%f\n' | sort)
mapfile -t embedded_migrations < <(cd "$EMBEDDED_MIGRATIONS_DIR" && find . -maxdepth 1 -type f -name '[0-9][0-9][0-9]_*.sql' -printf '%f\n' | sort)
[[ "${#versioned_migrations[@]}" -gt 0 ]] || { echo "No versioned migrations found" >&2; exit 1; }
[[ "${versioned_migrations[0]}" == "001_init.sql" ]] || { echo "Migration sequence must begin with 001_init.sql" >&2; exit 1; }
[[ "${versioned_migrations[*]}" == "${embedded_migrations[*]}" ]] || { echo "External and embedded migration file sets differ" >&2; exit 1; }

previous_number=0
for file in "${versioned_migrations[@]}"; do
  test -s "$MIGRATIONS_DIR/$file"
  test -s "$EMBEDDED_MIGRATIONS_DIR/$file"

  current_number="${file%%_*}"
  expected_number=$(printf '%03d' $((previous_number + 1)))
  if [[ "$current_number" != "$expected_number" ]]; then
    echo "Non-contiguous migration sequence: expected $expected_number but found $file" >&2
    exit 1
  fi
  previous_number=$((10#$current_number))

  if ! cmp -s "$MIGRATIONS_DIR/$file" "$EMBEDDED_MIGRATIONS_DIR/$file"; then
    echo "Migration file differs from embedded copy: $file" >&2
    exit 1
  fi

  if grep -Eiq '^[[:space:]]*DROP[[:space:]]+(TABLE|DATABASE)' "$MIGRATIONS_DIR/$file"; then
    echo "Unsafe DROP found in $file" >&2
    exit 1
  fi
done

if grep -R -n -E 'CREATE TABLE.*schema_migrations|INSERT INTO.*schema_migrations' "$MIGRATIONS_DIR" "$EMBEDDED_MIGRATIONS_DIR"; then
  echo "Versioned migration SQL must not manage schema_migrations; the Go runner owns version records" >&2
  exit 1
fi

grep -q "func ensureSchemaMigrations" "$ROOT/backend/internal/dbmigrate/runner.go"
grep -q "restock_times" "$MIGRATIONS_DIR/001_init.sql"
grep -q "custom_cron" "$MIGRATIONS_DIR/001_init.sql"
grep -q "calendar_policy" "$MIGRATIONS_DIR/001_init.sql"
grep -q "skip_reason" "$MIGRATIONS_DIR/001_init.sql"
grep -q "active_dedupe_key" "$MIGRATIONS_DIR/008_exchange_task_idempotency.sql"

[[ ! -e "$MIGRATIONS_DIR/init.sql" ]] || {
  echo "Non-versioned migrations/init.sql is forbidden; use 001_init.sql" >&2
  exit 1
}
for token in account_id resource_id idempotency_key attempt_count queued_at completed_at; do
  grep -q "\`$token\`" "$MIGRATIONS_DIR/010_operations.sql"
done
for token in refresh_token_hash token_version device_info expires_at revoked_at replaced_by_session_id last_used_at; do
  grep -q "\`$token\`" "$MIGRATIONS_DIR/011_refresh_sessions.sql"
done
for token in normalized_username normalized_email uk_users_normalized_username uk_users_normalized_email; do
  grep -q "$token" "$MIGRATIONS_DIR/012_normalized_user_identity.sql"
done
for token in message_id sequence expires_at acked_at uidx_ws_message_id idx_ws_user_sequence idx_ws_expires_at; do
  grep -q "$token" "$MIGRATIONS_DIR/013_websocket_delivery.sql"
done
for token in source_operation_id uk_exchange_tasks_source_operation; do
  grep -q "$token" "$MIGRATIONS_DIR/014_exchange_task_operation_id.sql"
done
grep -q "exchange_records" "$MIGRATIONS_DIR/017_exchange_record_rule_fk.sql"
for token in operations exchange_tasks execution_token AddColumnIfMissing; do
  grep -q "$token" "$MIGRATIONS_DIR/018_execution_fencing_tokens.sql"
done
for token in web_socket_sequences 'MAX(`sequence`)' 'ON DUPLICATE KEY UPDATE'; do
  grep -q "$token" "$MIGRATIONS_DIR/019_websocket_sequence_allocator.sql"
done
for token in 'DELETE older' uk_cloud_stats_account_date CreateUniqueIndexIfMissing; do
  grep -q "$token" "$MIGRATIONS_DIR/020_cloud_stats_account_date_unique.sql"
done

echo "migration smoke test passed (${#versioned_migrations[@]} versioned migrations checked)"
