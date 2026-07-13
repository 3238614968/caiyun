-- 014_exchange_task_operation_id.sql
-- Link immediate-exchange tasks to durable operations so redelivery reuses the same task.
DELIMITER $$
DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`$$
CREATE PROCEDURE `AddColumnIfNotExists`(IN p_table_name VARCHAR(100), IN p_column_name VARCHAR(100), IN p_column_def TEXT)
BEGIN
    SET @col_exists = (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = p_table_name AND column_name = p_column_name);
    IF @col_exists = 0 THEN
        SET @sql = CONCAT('ALTER TABLE `', p_table_name, '` ADD COLUMN `', p_column_name, '` ', p_column_def);
        PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    END IF;
END$$
DROP PROCEDURE IF EXISTS `CreateUniqueIndexIfNotExists`$$
CREATE PROCEDURE `CreateUniqueIndexIfNotExists`(IN p_table_name VARCHAR(100), IN p_index_name VARCHAR(100), IN p_index_def TEXT)
BEGIN
    SET @idx_exists = (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = p_table_name AND index_name = p_index_name);
    IF @idx_exists = 0 THEN
        SET @sql = CONCAT('CREATE UNIQUE INDEX `', p_index_name, '` ON `', p_table_name, '` (', p_index_def, ')');
        PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    END IF;
END$$
DELIMITER ;
CALL AddColumnIfNotExists('exchange_tasks', 'source_operation_id', 'CHAR(36) NULL COMMENT ''来源异步操作ID'' AFTER `id`');
CALL CreateUniqueIndexIfNotExists('exchange_tasks', 'uk_exchange_tasks_source_operation', '`source_operation_id`');
DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateUniqueIndexIfNotExists`;
