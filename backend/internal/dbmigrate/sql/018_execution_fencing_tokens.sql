-- 018_execution_fencing_tokens.sql
-- Persist Worker fencing tokens for durable operation and exchange-task leases.

DELIMITER $$

DROP PROCEDURE IF EXISTS `AddColumnIfMissing`$$
CREATE PROCEDURE `AddColumnIfMissing`(
    IN p_table_name VARCHAR(100),
    IN p_column_name VARCHAR(100),
    IN p_column_definition TEXT
)
BEGIN
    SET @column_exists = (
        SELECT COUNT(*)
        FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = p_table_name
          AND column_name = p_column_name
    );
    IF @column_exists = 0 THEN
        SET @sql = CONCAT('ALTER TABLE `', p_table_name, '` ADD COLUMN `', p_column_name, '` ', p_column_definition, ', ALGORITHM=INSTANT, LOCK=NONE');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

CALL AddColumnIfMissing('operations', 'execution_token', 'CHAR(36) NOT NULL DEFAULT '''' AFTER `status`');
CALL AddColumnIfMissing('exchange_tasks', 'execution_token', 'CHAR(36) NOT NULL DEFAULT '''' AFTER `status`');

DROP PROCEDURE IF EXISTS `AddColumnIfMissing`;
