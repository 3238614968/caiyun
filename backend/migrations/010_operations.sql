-- 010_operations.sql
-- Durable operation/outbox records for API-to-Worker asynchronous commands.

CREATE TABLE IF NOT EXISTS `operations` (
    `id` CHAR(36) NOT NULL,
    `user_id` BIGINT UNSIGNED NOT NULL,
    `operation_type` VARCHAR(64) NOT NULL,
    `status` VARCHAR(16) NOT NULL DEFAULT 'queued',
    `account_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `resource_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `payload` LONGTEXT NOT NULL,
    `idempotency_key` VARCHAR(191) NOT NULL,
    `attempt_count` INT NOT NULL DEFAULT 0,
    `error_summary` VARCHAR(512) NOT NULL DEFAULT '',
    `queued_at` DATETIME(3) NOT NULL,
    `started_at` DATETIME(3) NULL,
    `completed_at` DATETIME(3) NULL,
    `created_at` DATETIME(3) NOT NULL,
    `updated_at` DATETIME(3) NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uq_operations_user_idempotency` (`user_id`, `idempotency_key`),
    KEY `idx_operations_user_id` (`user_id`),
    KEY `idx_operations_operation_type` (`operation_type`),
    KEY `idx_operations_status_updated_at` (`status`, `updated_at`),
    KEY `idx_operations_account_id` (`account_id`),
    KEY `idx_operations_resource_id` (`resource_id`),
    KEY `idx_operations_queued_at` (`queued_at`),
    KEY `idx_operations_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;