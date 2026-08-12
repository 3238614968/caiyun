-- Keep the append-only archive schema compatible with the live exchange
-- records table and enforce the one-active-rule-per-cloud-account invariant.
-- 007 created archives with CREATE TABLE LIKE before 006/017 introduced and
-- changed exchange-record columns, so both the DDL and archive copy must be
-- explicit from this point forward.

DELIMITER $$

DROP PROCEDURE IF EXISTS `AddColumnIfMissing`$$
CREATE PROCEDURE `AddColumnIfMissing`(
    IN p_table_name VARCHAR(100),
    IN p_column_name VARCHAR(100),
    IN p_column_def TEXT
)
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = p_table_name
          AND column_name = p_column_name
    ) THEN
        SET @sql = CONCAT(
            'ALTER TABLE `', p_table_name, '` ADD COLUMN `', p_column_name, '` ', p_column_def
        );
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `MakeColumnNullableIfRequired`$$
CREATE PROCEDURE `MakeColumnNullableIfRequired`(
    IN p_table_name VARCHAR(100),
    IN p_column_name VARCHAR(100),
    IN p_column_def TEXT
)
BEGIN
    SET @is_nullable = NULL;
    SELECT is_nullable INTO @is_nullable
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = p_table_name
      AND column_name = p_column_name
    LIMIT 1;
    IF @is_nullable = 'NO' THEN
        SET @sql = CONCAT(
            'ALTER TABLE `', p_table_name, '` MODIFY COLUMN `', p_column_name, '` ', p_column_def
        );
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `CreateIndexIfMissing`$$
CREATE PROCEDURE `CreateIndexIfMissing`(
    IN p_table_name VARCHAR(100),
    IN p_index_name VARCHAR(100),
    IN p_columns TEXT,
    IN p_unique BOOLEAN
)
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = p_table_name
          AND index_name = p_index_name
    ) THEN
        IF p_unique THEN
            SET @sql = CONCAT(
                'ALTER TABLE `', p_table_name, '` ADD UNIQUE INDEX `', p_index_name,
                '` (', p_columns, ')'
            );
        ELSE
            SET @sql = CONCAT(
                'CREATE INDEX `', p_index_name, '` ON `', p_table_name,
                '` (', p_columns, ')'
            );
        END IF;
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

-- Archive parity for 006/017: runtime records use exchange_rule_id and leave
-- the historical exchange_account_id compatibility column empty.
CALL AddColumnIfMissing(
    'exchange_records_archive',
    'exchange_rule_id',
    'BIGINT UNSIGNED NULL COMMENT ''抢兑规则 ID'' AFTER `user_id`'
);
CALL MakeColumnNullableIfRequired(
    'exchange_records_archive',
    'exchange_account_id',
    'BIGINT UNSIGNED NULL COMMENT ''历史兼容抢兑账号 ID'''
);
UPDATE `exchange_records_archive`
SET `exchange_rule_id` = `exchange_account_id`
WHERE (`exchange_rule_id` IS NULL OR `exchange_rule_id` = 0)
  AND `exchange_account_id` IS NOT NULL;
CALL CreateIndexIfMissing(
    'exchange_records_archive',
    'idx_exchange_records_archive_rule_created',
    '`exchange_rule_id`, `created_at`',
    FALSE
);

-- Role changes and user deletion lock the admin set with `FOR UPDATE`.
-- This keeps that invariant check index-backed instead of scanning users.
CALL CreateIndexIfMissing(
    'users',
    'idx_users_role_id',
    '`role`, `id`',
    FALSE
);

-- Concurrent creates used to rely on a read-before-write check.  Keep the
-- newest active record if historical duplicates exist, then let a nullable
-- virtual key make the database enforce one active, non-deleted rule.
CALL AddColumnIfMissing(
    'exchange_rules',
    'active_account_id',
    'BIGINT UNSIGNED GENERATED ALWAYS AS (CASE WHEN `is_active` = 1 AND `deleted_at` IS NULL THEN `account_id` ELSE NULL END) VIRTUAL'
);
UPDATE `exchange_rules` AS older
INNER JOIN `exchange_rules` AS newer
  ON newer.`account_id` = older.`account_id`
 AND newer.`is_active` = 1
 AND newer.`deleted_at` IS NULL
 AND older.`is_active` = 1
 AND older.`deleted_at` IS NULL
 AND newer.`id` > older.`id`
SET older.`is_active` = FALSE;
CALL CreateIndexIfMissing(
    'exchange_rules',
    'uk_exchange_rules_active_account',
    '`active_account_id`',
    TRUE
);

DROP PROCEDURE IF EXISTS `AddColumnIfMissing`;
DROP PROCEDURE IF EXISTS `MakeColumnNullableIfRequired`;
DROP PROCEDURE IF EXISTS `CreateIndexIfMissing`;
