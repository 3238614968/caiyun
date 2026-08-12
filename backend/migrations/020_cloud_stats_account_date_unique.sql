-- 020_cloud_stats_account_date_unique.sql
-- Cloud-stat snapshots have one authoritative value per account/day.  Older
-- installations may predate that invariant.  Prefer a live snapshot over a
-- soft-deleted one, then retain the newest row within the same lifecycle
-- state before creating the index used by the atomic upsert path.

DELETE older
FROM `cloud_stats` AS older
INNER JOIN `cloud_stats` AS newer
 ON newer.`account_id` = older.`account_id`
 AND newer.`date` = older.`date`
 AND (
     (older.`deleted_at` IS NOT NULL AND newer.`deleted_at` IS NULL)
     OR (
         (
             (older.`deleted_at` IS NULL AND newer.`deleted_at` IS NULL)
             OR (older.`deleted_at` IS NOT NULL AND newer.`deleted_at` IS NOT NULL)
         )
         AND newer.`id` > older.`id`
     )
 );

DELIMITER $$

DROP PROCEDURE IF EXISTS `CreateUniqueIndexIfMissing`$$
CREATE PROCEDURE `CreateUniqueIndexIfMissing`(
    IN p_table_name VARCHAR(100),
    IN p_index_name VARCHAR(100),
    IN p_columns TEXT
)
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = p_table_name
          AND index_name = p_index_name
    ) THEN
        SET @sql = CONCAT(
            'ALTER TABLE `', p_table_name, '` ADD UNIQUE INDEX `', p_index_name,
            '` (', p_columns, '), ALGORITHM=INPLACE, LOCK=NONE'
        );
        PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

CALL CreateUniqueIndexIfMissing('cloud_stats', 'uk_cloud_stats_account_date', '`account_id`, `date`');

DROP PROCEDURE IF EXISTS `CreateUniqueIndexIfMissing`;
