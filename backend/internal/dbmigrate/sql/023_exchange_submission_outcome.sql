-- Persist submission safety independently of legacy task/record statuses.
-- Empty outcome denotes historical rows; never infer whether they submitted.

DELIMITER $$

DROP PROCEDURE IF EXISTS `AddExchangeOutcomeColumn`$$
CREATE PROCEDURE `AddExchangeOutcomeColumn`(IN p_table_name VARCHAR(100))
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = p_table_name
          AND column_name = 'outcome'
    ) THEN
        SET @sql = CONCAT(
            'ALTER TABLE `', p_table_name,
            '` ADD COLUMN `outcome` VARCHAR(16) NOT NULL DEFAULT '''' COMMENT ''提交结果：not_submitted/accepted/rejected/unknown'''
        );
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

CALL AddExchangeOutcomeColumn('exchange_tasks');
CALL AddExchangeOutcomeColumn('exchange_records');
CALL AddExchangeOutcomeColumn('exchange_records_archive');

DROP PROCEDURE IF EXISTS `AddExchangeOutcomeColumn`;
