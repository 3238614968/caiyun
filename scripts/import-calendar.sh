#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CSV_PATH="${1:-$ROOT/deploy/calendar/zh-cn-2026.csv}"

[[ -f "$CSV_PATH" ]] || { echo "Calendar CSV not found: $CSV_PATH" >&2; exit 1; }
command -v mysql >/dev/null 2>&1 || { echo "mysql client is required" >&2; exit 1; }

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-caiyun_app}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_NAME="${DB_NAME:-caiyun}"

mysql_args=(--local-infile=1 -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" "$DB_NAME")
if [[ -n "$DB_PASSWORD" ]]; then
  mysql_args=(-p"$DB_PASSWORD" "${mysql_args[@]}")
fi

mysql "${mysql_args[@]}" <<SQL
CREATE TABLE IF NOT EXISTS \`calendar_dates\` (
    \`date\` DATE NOT NULL COMMENT '日期',
    \`day_type\` VARCHAR(20) NOT NULL COMMENT 'holiday=节假日/休息日, workday=调休工作日',
    \`name\` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '节假日/调休名称',
    \`source\` VARCHAR(120) NOT NULL DEFAULT '' COMMENT '数据来源',
    \`created_at\` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    \`updated_at\` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (\`date\`),
    INDEX \`idx_calendar_dates_day_type\` (\`day_type\`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='节假日与调休工作日表';
LOAD DATA LOCAL INFILE '$(printf "%s" "$CSV_PATH" | sed "s/'/''/g")'
REPLACE INTO TABLE \`calendar_dates\`
CHARACTER SET utf8mb4
FIELDS TERMINATED BY ',' ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 LINES
(@date, @day_type, @name, @source)
SET
  \`date\` = STR_TO_DATE(TRIM(@date), '%Y-%m-%d'),
  \`day_type\` = LOWER(TRIM(@day_type)),
  \`name\` = TRIM(@name),
  \`source\` = IF(TRIM(@source) = '', 'csv_import', TRIM(@source));
SQL

echo "Imported calendar dates from $CSV_PATH"
