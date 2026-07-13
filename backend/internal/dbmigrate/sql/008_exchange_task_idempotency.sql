-- 008_exchange_task_idempotency.sql
-- 为抢兑任务补充数据库级活跃任务幂等约束，防止并发创建重复 pending/running 任务。

DELIMITER $$

DROP PROCEDURE IF EXISTS `AddGeneratedColumnIfNotExists`$$
CREATE PROCEDURE `AddGeneratedColumnIfNotExists`(
    IN p_table_name VARCHAR(100),
    IN p_column_name VARCHAR(100),
    IN p_column_def TEXT
)
BEGIN
    SET @col_exists = (
        SELECT COUNT(*)
        FROM information_schema.columns
        WHERE table_schema = DATABASE()
        AND table_name = p_table_name
        AND column_name = p_column_name
    );
    IF @col_exists = 0 THEN
        SET @sql = CONCAT('ALTER TABLE `', p_table_name, '` ADD COLUMN `', p_column_name, '` ', p_column_def);
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `CreateUniqueIndexIfNotExists`$$
CREATE PROCEDURE `CreateUniqueIndexIfNotExists`(
    IN p_table_name VARCHAR(100),
    IN p_index_name VARCHAR(100),
    IN p_index_def TEXT
)
BEGIN
    SET @idx_exists = (
        SELECT COUNT(*)
        FROM information_schema.statistics
        WHERE table_schema = DATABASE()
        AND table_name = p_table_name
        AND index_name = p_index_name
    );
    IF @idx_exists = 0 THEN
        SET @sql = CONCAT('CREATE UNIQUE INDEX `', p_index_name, '` ON `', p_table_name, '` (', p_index_def, ')');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `DropForeignKeyForColumnIfExists`$$
CREATE PROCEDURE `DropForeignKeyForColumnIfExists`(
    IN p_table_name VARCHAR(100),
    IN p_column_name VARCHAR(100)
)
BEGIN
    SET @fk_name = NULL;
    SELECT constraint_name INTO @fk_name
    FROM information_schema.key_column_usage
    WHERE table_schema = DATABASE()
    AND table_name = p_table_name
    AND column_name = p_column_name
    AND referenced_table_name IS NOT NULL
    LIMIT 1;
    IF @fk_name IS NOT NULL THEN
        SET @sql = CONCAT('ALTER TABLE `', p_table_name, '` DROP FOREIGN KEY `', @fk_name, '`');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `MakeColumnNullableIfNotExists`$$
CREATE PROCEDURE `MakeColumnNullableIfNotExists`(
    IN p_table_name VARCHAR(100),
    IN p_column_name VARCHAR(100),
    IN p_column_type TEXT
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
        SET @sql = CONCAT('ALTER TABLE `', p_table_name, '` MODIFY COLUMN `', p_column_name, '` ', p_column_type, ' NULL');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `CreateForeignKeyIfNotExists`$$
CREATE PROCEDURE `CreateForeignKeyIfNotExists`(
    IN p_table_name VARCHAR(100),
    IN p_column_name VARCHAR(100),
    IN p_constraint_name VARCHAR(100),
    IN p_referenced_table VARCHAR(100),
    IN p_referenced_column VARCHAR(100)
)
BEGIN
    SET @fk_exists = (
        SELECT COUNT(*)
        FROM information_schema.key_column_usage
        WHERE table_schema = DATABASE()
        AND table_name = p_table_name
        AND column_name = p_column_name
        AND referenced_table_name IS NOT NULL
    );
    IF @fk_exists = 0 THEN
        SET @sql = CONCAT(
            'ALTER TABLE `', p_table_name, '` ADD CONSTRAINT `', p_constraint_name,
            '` FOREIGN KEY (`', p_column_name, '`) REFERENCES `', p_referenced_table,
            '` (`', p_referenced_column, '`) ON DELETE CASCADE'
        );
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

-- 历史数据治理：同一用户/抢兑规则/商品/任务类型仅保留最早的一条活跃任务，后续重复任务标记失败，避免唯一索引创建失败。
UPDATE `exchange_tasks` t
JOIN (
    SELECT id
    FROM (
        SELECT
            id,
            ROW_NUMBER() OVER (
                PARTITION BY user_id, exchange_rule_id, product_id, task_type
                ORDER BY id ASC
            ) AS rn
        FROM `exchange_tasks`
        WHERE deleted_at IS NULL
        AND status IN ('pending', 'running')
    ) ranked
    WHERE rn > 1
) dup ON dup.id = t.id
SET
    t.status = 'failed',
    t.skip_reason = 'duplicate_active_task_archived_by_migration',
    t.last_result = '重复活跃抢兑任务已由迁移归档为失败状态',
    t.updated_at = NOW();

-- `exchange_account_id` 是 001 基线遗留的兼容影子列。006 将父表重命名后，
-- 某些 MySQL 版本会保留指向兼容视图的外键，进而拒绝重建表。它不再被运行时代码读取，
-- 因此在新增 VIRTUAL 生成列前移除这条旧约束。
CALL DropForeignKeyForColumnIfExists('exchange_tasks', 'exchange_account_id');
CALL MakeColumnNullableIfNotExists('exchange_tasks', 'exchange_account_id', 'BIGINT UNSIGNED');

CALL AddGeneratedColumnIfNotExists(
    'exchange_tasks',
    'active_dedupe_key',
    'VARCHAR(255) GENERATED ALWAYS AS (CASE WHEN `deleted_at` IS NULL AND `status` IN (''pending'', ''running'') THEN CONCAT(`user_id`, '':'', `exchange_rule_id`, '':'', `product_id`, '':'', `task_type`) ELSE NULL END) VIRTUAL'
);

CALL CreateUniqueIndexIfNotExists('exchange_tasks', 'uk_exchange_tasks_active_dedupe', '`active_dedupe_key`');
CALL CreateForeignKeyIfNotExists('exchange_tasks', 'exchange_rule_id', 'fk_exchange_tasks_exchange_rule_id', 'exchange_rules', 'id');

DROP PROCEDURE IF EXISTS `AddGeneratedColumnIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateUniqueIndexIfNotExists`;
DROP PROCEDURE IF EXISTS `DropForeignKeyForColumnIfExists`;
DROP PROCEDURE IF EXISTS `MakeColumnNullableIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateForeignKeyIfNotExists`;
