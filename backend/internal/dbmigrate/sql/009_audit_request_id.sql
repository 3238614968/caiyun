-- 009_audit_request_id.sql
-- Add request_id to audit logs so HTTP access logs, error logs and audit records can be correlated.

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

CALL AddColumnIfNotExists('audit_logs', 'request_id', 'VARCHAR(128) NULL COMMENT ''请求追踪ID'' AFTER `path`');
CALL CreateIndexIfNotExists('audit_logs', 'idx_audit_logs_request_id', '`request_id`');

DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateIndexIfNotExists`;
