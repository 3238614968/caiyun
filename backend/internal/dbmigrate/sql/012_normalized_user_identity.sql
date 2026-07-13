-- 012_normalized_user_identity.sql
-- Normalize login identifiers independently of database collation and enforce
-- database-level uniqueness. Empty email addresses remain NULL.

DELIMITER $$

DROP PROCEDURE IF EXISTS `AddIdentityColumnIfNotExists`$$
CREATE PROCEDURE `AddIdentityColumnIfNotExists`(
    IN p_column_name VARCHAR(100),
    IN p_column_def TEXT
)
BEGIN
    SET @col_exists = (
        SELECT COUNT(*)
        FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = 'users'
          AND column_name = p_column_name
    );
    IF @col_exists = 0 THEN
        SET @sql = CONCAT('ALTER TABLE `users` ADD COLUMN `', p_column_name, '` ', p_column_def);
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `CreateIdentityUniqueIndexIfNotExists`$$
CREATE PROCEDURE `CreateIdentityUniqueIndexIfNotExists`(
    IN p_index_name VARCHAR(100),
    IN p_index_def TEXT
)
BEGIN
    SET @idx_exists = (
        SELECT COUNT(*)
        FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = 'users'
          AND index_name = p_index_name
    );
    IF @idx_exists = 0 THEN
        SET @sql = CONCAT('CREATE UNIQUE INDEX `', p_index_name, '` ON `users` (', p_index_def, ')');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `AssertNormalizedIdentityUnique`$$
CREATE PROCEDURE `AssertNormalizedIdentityUnique`()
BEGIN
    IF EXISTS (
        SELECT 1
        FROM users
        WHERE deleted_at IS NULL
        GROUP BY normalized_username
        HAVING normalized_username IS NOT NULL AND COUNT(*) > 1
        LIMIT 1
    ) THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'duplicate normalized usernames exist; resolve them before retrying migration 012';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM users
        WHERE deleted_at IS NULL AND normalized_email IS NOT NULL
        GROUP BY normalized_email
        HAVING COUNT(*) > 1
        LIMIT 1
    ) THEN
        SIGNAL SQLSTATE '45000'
            SET MESSAGE_TEXT = 'duplicate normalized emails exist; resolve them before retrying migration 012';
    END IF;
END$$

DELIMITER ;

CALL AddIdentityColumnIfNotExists(
    'normalized_username',
    'VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER `username`'
);
CALL AddIdentityColumnIfNotExists(
    'normalized_email',
    'VARCHAR(254) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NULL AFTER `email`'
);

UPDATE users
SET normalized_username = CASE
        WHEN deleted_at IS NULL THEN LOWER(TRIM(username))
        ELSE CONCAT('deleted-', id)
    END,
    normalized_email = CASE
        WHEN deleted_at IS NOT NULL OR NULLIF(TRIM(email), '') IS NULL THEN NULL
        WHEN LOCATE('@', TRIM(email)) > 0 THEN CONCAT(
            SUBSTRING_INDEX(TRIM(email), '@', 1),
            '@',
            LOWER(SUBSTRING_INDEX(TRIM(email), '@', -1))
        )
        ELSE TRIM(email)
    END
WHERE normalized_username IS NULL
   OR normalized_username <> CASE
        WHEN deleted_at IS NULL THEN LOWER(TRIM(username))
        ELSE CONCAT('deleted-', id)
   END
   OR (
        normalized_email IS NULL AND deleted_at IS NULL AND NULLIF(TRIM(email), '') IS NOT NULL
   )
   OR (
        normalized_email IS NOT NULL AND normalized_email <> CASE
            WHEN deleted_at IS NOT NULL OR NULLIF(TRIM(email), '') IS NULL THEN NULL
            WHEN LOCATE('@', TRIM(email)) > 0 THEN CONCAT(
                SUBSTRING_INDEX(TRIM(email), '@', 1),
                '@',
                LOWER(SUBSTRING_INDEX(TRIM(email), '@', -1))
            )
            ELSE TRIM(email)
        END
   );

CALL AssertNormalizedIdentityUnique();
CALL CreateIdentityUniqueIndexIfNotExists(
    'uk_users_normalized_username',
    '`normalized_username`'
);
CALL CreateIdentityUniqueIndexIfNotExists(
    'uk_users_normalized_email',
    '`normalized_email`'
);

DROP PROCEDURE IF EXISTS `AssertNormalizedIdentityUnique`;
DROP PROCEDURE IF EXISTS `CreateIdentityUniqueIndexIfNotExists`;
DROP PROCEDURE IF EXISTS `AddIdentityColumnIfNotExists`;
