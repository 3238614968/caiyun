-- Keep per-command exchange completion separate from future task schedules.
-- The receipt is also the durable submission boundary for monthly accounts.

CREATE TABLE IF NOT EXISTS `exchange_execution_receipts` (
  `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  `operation_id` VARCHAR(36) NOT NULL COMMENT '异步操作 ID',
  `resource_id` BIGINT UNSIGNED NOT NULL COMMENT '任务或兑换账号 ID',
  `resource_kind` VARCHAR(24) NOT NULL COMMENT 'exchange_task/exchange_account',
  `user_id` BIGINT UNSIGNED NOT NULL COMMENT '资源所属用户 ID',
  `product_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '商品 ID',
  `outcome` VARCHAR(16) NOT NULL DEFAULT '' COMMENT 'not_submitted/accepted/rejected/unknown',
  `message` TEXT COMMENT '已脱敏结果',
  `execution_time_ms` INT NOT NULL DEFAULT 0,
  `execution_token` VARCHAR(36) NOT NULL DEFAULT '',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `completed_at` DATETIME(3) NULL,
  UNIQUE KEY `uk_exchange_receipt_operation_resource` (`operation_id`, `resource_id`, `resource_kind`),
  KEY `idx_exchange_receipt_user` (`user_id`, `updated_at`),
  CONSTRAINT `fk_exchange_receipt_operation` FOREIGN KEY (`operation_id`) REFERENCES `operations` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
