-- 017_exchange_record_rule_fk.sql
-- exchange_records 在 006 中新增了 exchange_rule_id，但旧 exchange_account_id
-- 兼容列仍可能是 NOT NULL。运行时模型只写 exchange_rule_id，旧列会导致
-- 结果记录写入失败，从而阻断任务状态提交和前端推送。

DELIMITER $$

DROP PROCEDURE IF EXISTS `DropLegacyExchangeRecordForeignKey`$$
CREATE PROCEDURE `DropLegacyExchangeRecordForeignKey`()
BEGIN
    SET @fk_name = NULL;
    SELECT constraint_name INTO @fk_name
    FROM information_schema.key_column_usage
    WHERE table_schema = DATABASE()
      AND table_name = 'exchange_records'
      AND column_name = 'exchange_account_id'
      AND referenced_table_name IS NOT NULL
    LIMIT 1;
    IF @fk_name IS NOT NULL THEN
        SET @sql = CONCAT('ALTER TABLE `exchange_records` DROP FOREIGN KEY `', @fk_name, '`');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `MakeLegacyExchangeRecordColumnNullable`$$
CREATE PROCEDURE `MakeLegacyExchangeRecordColumnNullable`()
BEGIN
    SET @is_nullable = NULL;
    SELECT is_nullable INTO @is_nullable
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'exchange_records'
      AND column_name = 'exchange_account_id'
    LIMIT 1;
    IF @is_nullable = 'NO' THEN
        ALTER TABLE `exchange_records` MODIFY COLUMN `exchange_account_id` BIGINT UNSIGNED NULL;
    END IF;
END$$

DELIMITER ;

CALL DropLegacyExchangeRecordForeignKey();
CALL MakeLegacyExchangeRecordColumnNullable();

DROP PROCEDURE IF EXISTS `DropLegacyExchangeRecordForeignKey`;
DROP PROCEDURE IF EXISTS `MakeLegacyExchangeRecordColumnNullable`;
