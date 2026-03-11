-- 增强兑换中心功能迁移
-- 执行时间：2026-02-20

-- 1. 为 exchange_tasks 表增加优先级、分组和重试相关字段
ALTER TABLE `exchange_tasks`
ADD COLUMN `priority` INT DEFAULT 0 COMMENT '优先级：0-普通，1-重要，2-紧急' AFTER `status`,
ADD COLUMN `task_group` VARCHAR(50) DEFAULT '' COMMENT '任务分组标识' AFTER `priority`,
ADD COLUMN `timeout_seconds` INT DEFAULT 30 COMMENT '超时时间（秒）' AFTER `task_group`,
ADD COLUMN `max_retries` INT DEFAULT 3 COMMENT '最大重试次数' AFTER `timeout_seconds`,
ADD COLUMN `retry_count` INT DEFAULT 0 COMMENT '已重试次数' AFTER `max_retries`,
ADD COLUMN `last_retry_at` DATETIME NULL COMMENT '最后重试时间' AFTER `retry_count`;

-- 2. 为关键字段增加索引，提高查询性能
CREATE INDEX `idx_exchange_tasks_priority_status` ON `exchange_tasks` (`priority`, `status`);
CREATE INDEX `idx_exchange_tasks_task_group` ON `exchange_tasks` (`task_group`);
CREATE INDEX `idx_exchange_tasks_timeout` ON `exchange_tasks` (`timeout_seconds`, `status`);

-- 3. 为 exchange_accounts 表增加并发配置字段
ALTER TABLE `exchange_accounts`
ADD COLUMN `max_concurrency` INT DEFAULT 1 COMMENT '该账号最大并发数' AFTER `is_active`,
ADD COLUMN `retry_interval` INT DEFAULT 5 COMMENT '失败重试间隔（秒）' AFTER `max_concurrency`;

-- 4. 为 products 表增加库存状态字段
ALTER TABLE `products`
ADD COLUMN `stock_status` VARCHAR(20) DEFAULT 'unknown' COMMENT '库存状态：unknown, available, sold_out' AFTER `daily_remainder_count`,
ADD COLUMN `last_stock_check` DATETIME NULL COMMENT '最后库存检查时间' AFTER `stock_status`;

-- 5. 新增兑换中心相关系统配置（字段名统一使用 key_name / key_value）
INSERT IGNORE INTO `system_configs` (`key_name`, `key_value`, `description`) VALUES
('exchange_enable_priority', 'true', '是否启用任务优先级'),
('exchange_default_timeout', '30', '默认抢兑超时时间（秒）'),
('exchange_max_global_concurrency', '50', '全局最大并发数限制'),
('exchange_auto_retry_failed', 'true', '自动重试失败任务'),
('exchange_log_retention_days', '30', '日志保留天数');

-- 6. 创建兑换任务执行历史表（用于统计和分析）
CREATE TABLE IF NOT EXISTS `exchange_task_history` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `task_id` INT NOT NULL COMMENT '任务 ID',
    `account_id` INT NOT NULL COMMENT '账号 ID',
    `prize_id` VARCHAR(100) NOT NULL COMMENT '商品 ID',
    `action` VARCHAR(50) NOT NULL COMMENT '操作类型：exchange, retry, timeout, cancel',
    `result` VARCHAR(20) NOT NULL COMMENT '结果：success, failed, timeout',
    `message` TEXT COMMENT '详细信息',
    `duration_ms` INT DEFAULT 0 COMMENT '执行时长（毫秒）',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    INDEX `idx_task_id` (`task_id`),
    INDEX `idx_account_id` (`account_id`),
    INDEX `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='兑换任务执行历史表';
