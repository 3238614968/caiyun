#!/usr/bin/env bash
set -euo pipefail

DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-caiyun_app}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_NAME="${DB_NAME:-caiyun}"
ARCHIVE_TASK_LOGS_BEFORE_DAYS="${ARCHIVE_TASK_LOGS_BEFORE_DAYS:-90}"
ARCHIVE_EXCHANGE_RECORDS_BEFORE_DAYS="${ARCHIVE_EXCHANGE_RECORDS_BEFORE_DAYS:-180}"
ARCHIVE_BATCH_SIZE="${ARCHIVE_BATCH_SIZE:-1000}"

MYSQL_BASE=(mysql -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USER}" "${DB_NAME}" --default-character-set=utf8mb4 --batch --skip-column-names)

run_sql() {
  local sql="$1"
  # MYSQL_PWD keeps the credential out of argv and process listings.  The
  # process-local environment is inherited only by this mysql invocation.
  MYSQL_PWD="${DB_PASSWORD}" "${MYSQL_BASE[@]}" -e "$sql"
}

archive_table() {
  local source_table="$1"
  local archive_table="$2"
  local cutoff_days="$3"
  local cutoff_expr="DATE_SUB(NOW(), INTERVAL ${cutoff_days} DAY)"
  local moved=0

  echo "[archive] ${source_table} -> ${archive_table}, cutoff=${cutoff_days} days, batch=${ARCHIVE_BATCH_SIZE}"
  while true; do
    local count
    count="$(run_sql "SELECT COUNT(*) FROM (SELECT id FROM ${source_table} WHERE created_at < ${cutoff_expr} ORDER BY id LIMIT ${ARCHIVE_BATCH_SIZE}) AS t;")"
    count="${count//[[:space:]]/}"
    if [[ -z "${count}" || "${count}" == "0" ]]; then
      break
    fi

    # Copy and deletion are one transaction.  A duplicate destination ID is
    # an explicit error that rolls the transaction back; INSERT IGNORE would
    # otherwise permit a later DELETE to silently discard source history.
    local deleted
    deleted="$(run_sql "START TRANSACTION; INSERT INTO ${archive_table} SELECT * FROM ${source_table} WHERE id IN (SELECT id FROM (SELECT id FROM ${source_table} WHERE created_at < ${cutoff_expr} ORDER BY id LIMIT ${ARCHIVE_BATCH_SIZE}) AS picked); DELETE FROM ${source_table} WHERE id IN (SELECT id FROM (SELECT id FROM ${source_table} WHERE created_at < ${cutoff_expr} ORDER BY id LIMIT ${ARCHIVE_BATCH_SIZE}) AS picked); SELECT ROW_COUNT(); COMMIT;")"
    deleted="${deleted//[[:space:]]/}"
    if [[ -z "${deleted}" || ! "${deleted}" =~ ^[0-9]+$ ]]; then
      echo "[archive] ${source_table}: invalid moved row count: ${deleted:-<empty>}" >&2
      exit 1
    fi
    moved=$((moved + deleted))
    echo "[archive] ${source_table}: moved ${deleted}, total ${moved}"
  done
}

archive_table "task_logs" "task_logs_archive" "${ARCHIVE_TASK_LOGS_BEFORE_DAYS}"
archive_table "exchange_records" "exchange_records_archive" "${ARCHIVE_EXCHANGE_RECORDS_BEFORE_DAYS}"

echo "[archive] done"
