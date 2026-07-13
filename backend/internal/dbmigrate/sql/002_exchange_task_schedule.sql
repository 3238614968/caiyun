-- 002_exchange_task_schedule.sql
-- 抢兑任务支持任务级时间与补货周期，幂等执行。

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

CALL AddColumnIfNotExists('exchange_tasks', 'scheduled_exchange_time', 'TIME NULL COMMENT ''任务级指定抢兑时间，为空则使用账号规则时间'' AFTER `max_attempts`');
CALL AddColumnIfNotExists('exchange_tasks', 'restock_cycle', 'VARCHAR(20) DEFAULT ''daily'' COMMENT ''补货周期: daily/weekly/monthly/once'' AFTER `scheduled_exchange_time`');
CALL AddColumnIfNotExists('exchange_tasks', 'restock_weekday', 'TINYINT NULL COMMENT ''weekly 周期星期，0=周日'' AFTER `restock_cycle`');
CALL AddColumnIfNotExists('exchange_tasks', 'restock_day_of_month', 'TINYINT NULL COMMENT ''monthly 周期日期'' AFTER `restock_weekday`');
CALL CreateIndexIfNotExists('exchange_tasks', 'idx_exchange_tasks_schedule', '`scheduled_exchange_time`, `restock_cycle`, `status`');

DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateIndexIfNotExists`;
