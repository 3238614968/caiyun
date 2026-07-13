-- 004_exchange_schedule_strategy.sql
-- 抢兑任务支持多补货时间点、自定义 cron、工作日/节假日策略和最近跳过原因。

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

DELIMITER ;

CALL AddColumnIfNotExists('exchange_tasks', 'restock_times', 'TEXT COMMENT ''补货时间点列表，逗号分隔 HH:MM[:SS]'' AFTER `restock_day_of_month`');
CALL AddColumnIfNotExists('exchange_tasks', 'custom_cron', 'VARCHAR(120) DEFAULT '''' COMMENT ''自定义 cron，优先于固定时间/补货时间'' AFTER `restock_times`');
CALL AddColumnIfNotExists('exchange_tasks', 'calendar_policy', 'VARCHAR(20) DEFAULT ''all'' COMMENT ''日历策略: all/workday/holiday'' AFTER `custom_cron`');
CALL AddColumnIfNotExists('exchange_tasks', 'holiday_dates', 'TEXT COMMENT ''额外节假日 YYYY-MM-DD，逗号分隔'' AFTER `calendar_policy`');
CALL AddColumnIfNotExists('exchange_tasks', 'workday_dates', 'TEXT COMMENT ''调休工作日 YYYY-MM-DD，逗号分隔'' AFTER `holiday_dates`');
CALL AddColumnIfNotExists('exchange_tasks', 'skip_reason', 'VARCHAR(255) DEFAULT '''' COMMENT ''最近一次调度跳过原因'' AFTER `workday_dates`');
CALL CreateIndexIfNotExists('exchange_tasks', 'idx_exchange_tasks_strategy', '`restock_cycle`, `calendar_policy`, `status`');

DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateIndexIfNotExists`;
