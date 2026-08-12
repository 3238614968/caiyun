-- Authentication endpoints are deliberately audited before an authenticated
-- user exists.  Audit rows must therefore accept a NULL actor and survive
-- user deletion instead of being constrained/cascaded through users.

DELIMITER $$

DROP PROCEDURE IF EXISTS `DropAuditLogUserForeignKey`$$
CREATE PROCEDURE `DropAuditLogUserForeignKey`()
BEGIN
    SET @fk_name = NULL;
    SELECT constraint_name INTO @fk_name
    FROM information_schema.key_column_usage
    WHERE table_schema = DATABASE()
      AND table_name = 'audit_logs'
      AND column_name = 'user_id'
      AND referenced_table_name IS NOT NULL
    LIMIT 1;
    IF @fk_name IS NOT NULL THEN
        SET @sql = CONCAT('ALTER TABLE `audit_logs` DROP FOREIGN KEY `', @fk_name, '`');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `MakeAuditLogUserIDNullable`$$
CREATE PROCEDURE `MakeAuditLogUserIDNullable`()
BEGIN
    SET @is_nullable = NULL;
    SELECT is_nullable INTO @is_nullable
    FROM information_schema.columns
    WHERE table_schema = DATABASE()
      AND table_name = 'audit_logs'
      AND column_name = 'user_id'
    LIMIT 1;
    IF @is_nullable = 'NO' THEN
        ALTER TABLE `audit_logs` MODIFY COLUMN `user_id` BIGINT UNSIGNED NULL;
    END IF;
END$$

DELIMITER ;

CALL DropAuditLogUserForeignKey();
CALL MakeAuditLogUserIDNullable();

DROP PROCEDURE IF EXISTS `DropAuditLogUserForeignKey`;
DROP PROCEDURE IF EXISTS `MakeAuditLogUserIDNullable`;
