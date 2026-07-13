-- 006_exchange_rule_physical_rename.sql
-- ExchangeAccount -> ExchangeRule 最终阶段：物理表名/关联字段统一为 exchange_rules / exchange_rule_id。
-- 为便于回滚，旧 exchange_account_id 字段保留为兼容影子列；旧 exchange_accounts 名称在可能时创建为兼容视图。

DELIMITER $$

DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`$$
CREATE PROCEDURE `AddColumnIfNotExists`(
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

DROP PROCEDURE IF EXISTS `CreateIndexIfNotExists`$$
CREATE PROCEDURE `CreateIndexIfNotExists`(
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
        SET @sql = CONCAT('CREATE INDEX `', p_index_name, '` ON `', p_table_name, '` (', p_index_def, ')');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `BackfillColumnIfExists`$$
CREATE PROCEDURE `BackfillColumnIfExists`(
    IN p_table_name VARCHAR(100),
    IN p_new_column VARCHAR(100),
    IN p_old_column VARCHAR(100)
)
BEGIN
    SET @new_col_exists = (
        SELECT COUNT(*) FROM information_schema.columns
        WHERE table_schema = DATABASE() AND table_name = p_table_name AND column_name = p_new_column
    );
    SET @old_col_exists = (
        SELECT COUNT(*) FROM information_schema.columns
        WHERE table_schema = DATABASE() AND table_name = p_table_name AND column_name = p_old_column
    );
    IF @new_col_exists = 1 AND @old_col_exists = 1 THEN
        SET @sql = CONCAT(
            'UPDATE `', p_table_name, '` SET `', p_new_column, '` = `', p_old_column, '` ',
            'WHERE (`', p_new_column, '` IS NULL OR `', p_new_column, '` = 0) ',
            'AND `', p_old_column, '` IS NOT NULL'
        );
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `RenameExchangeAccountsToRules`$$
CREATE PROCEDURE `RenameExchangeAccountsToRules`()
BEGIN
    SET @old_table_exists = (
        SELECT COUNT(*) FROM information_schema.tables
        WHERE table_schema = DATABASE() AND table_name = 'exchange_accounts'
    );
    SET @old_view_exists = (
        SELECT COUNT(*) FROM information_schema.views
        WHERE table_schema = DATABASE() AND table_name = 'exchange_accounts'
    );
    SET @new_table_exists = (
        SELECT COUNT(*) FROM information_schema.tables
        WHERE table_schema = DATABASE() AND table_name = 'exchange_rules'
    );

    IF @new_table_exists = 0 AND @old_table_exists = 1 THEN
        RENAME TABLE `exchange_accounts` TO `exchange_rules`;
        SET @new_table_exists = 1;
        SET @old_table_exists = 0;
    END IF;

    IF @new_table_exists = 1 AND @old_table_exists = 0 AND @old_view_exists = 0 THEN
        SET @sql = 'CREATE OR REPLACE VIEW `exchange_accounts` AS SELECT * FROM `exchange_rules`';
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

CALL RenameExchangeAccountsToRules();

CALL AddColumnIfNotExists('exchange_tasks', 'exchange_rule_id', 'BIGINT UNSIGNED NULL COMMENT ''抢兑规则 ID'' AFTER `user_id`');
CALL BackfillColumnIfExists('exchange_tasks', 'exchange_rule_id', 'exchange_account_id');
CALL CreateIndexIfNotExists('exchange_tasks', 'idx_exchange_tasks_exchange_rule_id', '`exchange_rule_id`');

CALL AddColumnIfNotExists('exchange_records', 'exchange_rule_id', 'BIGINT UNSIGNED NULL COMMENT ''抢兑规则 ID'' AFTER `user_id`');
CALL BackfillColumnIfExists('exchange_records', 'exchange_rule_id', 'exchange_account_id');
CALL CreateIndexIfNotExists('exchange_records', 'idx_exchange_records_exchange_rule_id', '`exchange_rule_id`');

DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateIndexIfNotExists`;
DROP PROCEDURE IF EXISTS `BackfillColumnIfExists`;
DROP PROCEDURE IF EXISTS `RenameExchangeAccountsToRules`;
