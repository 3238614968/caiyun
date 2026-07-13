-- 016_exchange_task_schedule_dedupe.sql
-- 支持同一抢兑账号在不同兑换配置下创建同一商品任务；仅阻止完全相同的活跃配置。
-- 旧键只包含账号/商品/任务类型，导致不同时间点的任务也被误判为重复。

DELIMITER $$

DROP PROCEDURE IF EXISTS `DropIndexIfExists`$$
CREATE PROCEDURE `DropIndexIfExists`(IN p_table_name VARCHAR(100), IN p_index_name VARCHAR(100))
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = p_table_name
          AND index_name = p_index_name
    ) THEN
        SET @sql = CONCAT('ALTER TABLE `', p_table_name, '` DROP INDEX `', p_index_name, '`');
        PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    END IF;
END$$

DROP PROCEDURE IF EXISTS `DropColumnIfExists`$$
CREATE PROCEDURE `DropColumnIfExists`(IN p_table_name VARCHAR(100), IN p_column_name VARCHAR(100))
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = p_table_name
          AND column_name = p_column_name
    ) THEN
        SET @sql = CONCAT('ALTER TABLE `', p_table_name, '` DROP COLUMN `', p_column_name, '`');
        PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

CALL DropIndexIfExists('exchange_tasks', 'uk_exchange_tasks_active_dedupe');
CALL DropColumnIfExists('exchange_tasks', 'active_dedupe_key');

ALTER TABLE `exchange_tasks`
ADD COLUMN `active_dedupe_key` CHAR(64)
GENERATED ALWAYS AS (
    CASE
        WHEN `deleted_at` IS NULL AND `status` IN ('pending', 'running') THEN SHA2(CONCAT_WS('|',
            `user_id`,
            `exchange_rule_id`,
            `product_id`,
            COALESCE(NULLIF(`task_type`, ''), 'fixed'),
            COALESCE(NULLIF(`scheduled_exchange_time`, ''), 'rule_time'),
            COALESCE(NULLIF(`restock_cycle`, ''), 'daily'),
            COALESCE(`restock_weekday`, -1),
            COALESCE(`restock_day_of_month`, -1),
            COALESCE(NULLIF(`restock_times`, ''), '-'),
            COALESCE(NULLIF(`custom_cron`, ''), '-'),
            COALESCE(NULLIF(`calendar_policy`, ''), 'all'),
            COALESCE(NULLIF(`holiday_dates`, ''), '-'),
            COALESCE(NULLIF(`workday_dates`, ''), '-')
        ), 256)
        ELSE NULL
    END
) VIRTUAL;

CREATE UNIQUE INDEX `uk_exchange_tasks_active_dedupe` ON `exchange_tasks` (`active_dedupe_key`);

DROP PROCEDURE IF EXISTS `DropIndexIfExists`;
DROP PROCEDURE IF EXISTS `DropColumnIfExists`;
