-- 015_exchange_task_rule_fk.sql
-- 将任务关联统一到 exchange_rules，清理 001/006 遗留的 exchange_account_id 外键。

DELIMITER $$

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

CALL DropForeignKeyForColumnIfExists('exchange_tasks', 'exchange_account_id');
CALL MakeColumnNullableIfNotExists('exchange_tasks', 'exchange_account_id', 'BIGINT UNSIGNED');
CALL DropForeignKeyForColumnIfExists('exchange_tasks', 'exchange_rule_id');
CALL CreateForeignKeyIfNotExists('exchange_tasks', 'exchange_rule_id', 'fk_exchange_tasks_exchange_rule_id', 'exchange_rules', 'id');

DROP PROCEDURE IF EXISTS `DropForeignKeyForColumnIfExists`;
DROP PROCEDURE IF EXISTS `MakeColumnNullableIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateForeignKeyIfNotExists`;