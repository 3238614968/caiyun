-- 007_log_archive_and_indexes.sql
-- 为高频日志与抢兑记录补充冷热分层表及联合索引，降低趋势统计/列表查询压力。

DELIMITER $$

DROP PROCEDURE IF EXISTS `CreateTableLikeIfNotExists`$$
CREATE PROCEDURE `CreateTableLikeIfNotExists`(
    IN p_new_table VARCHAR(100),
    IN p_source_table VARCHAR(100)
)
BEGIN
    SET @tbl_exists = (
        SELECT COUNT(*)
        FROM information_schema.tables
        WHERE table_schema = DATABASE()
        AND table_name = p_new_table
    );
    IF @tbl_exists = 0 THEN
        SET @sql = CONCAT('CREATE TABLE `', p_new_table, '` LIKE `', p_source_table, '`');
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

CALL CreateTableLikeIfNotExists('task_logs_archive', 'task_logs');
CALL CreateTableLikeIfNotExists('exchange_records_archive', 'exchange_records');

CALL CreateIndexIfNotExists('task_logs', 'idx_task_logs_account_task_created', '`account_id`, `task_type`, `created_at`');
CALL CreateIndexIfNotExists('task_logs', 'idx_task_logs_user_created', '`user_id`, `created_at`');
CALL CreateIndexIfNotExists('exchange_tasks', 'idx_exchange_tasks_status_schedule', '`status`, `scheduled_exchange_time`');
CALL CreateIndexIfNotExists('exchange_tasks', 'idx_exchange_tasks_rule_status', '`exchange_rule_id`, `status`');
CALL CreateIndexIfNotExists('exchange_records', 'idx_exchange_records_rule_created', '`exchange_rule_id`, `created_at`');
CALL CreateIndexIfNotExists('exchange_records', 'idx_exchange_records_status_created', '`status`, `created_at`');
CALL CreateIndexIfNotExists('cloud_stats', 'idx_cloud_stats_account_date', '`account_id`, `date`');
CALL CreateIndexIfNotExists('cloud_stats', 'idx_cloud_stats_user_date', '`user_id`, `date`');

DROP PROCEDURE IF EXISTS `CreateTableLikeIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateIndexIfNotExists`;
