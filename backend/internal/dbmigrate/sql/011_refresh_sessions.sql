-- 011_refresh_sessions.sql
-- Rotating refresh sessions. Only SHA-256 token digests are stored.

CREATE TABLE IF NOT EXISTS `refresh_sessions` (
    `id` VARCHAR(36) NOT NULL,
    `user_id` BIGINT UNSIGNED NOT NULL,
    `refresh_token_hash` CHAR(64) NOT NULL,
    `token_version` INT NOT NULL DEFAULT 0,
    `device_info` VARCHAR(255) NULL,
    `expires_at` DATETIME(6) NOT NULL,
    `revoked_at` DATETIME(6) NULL,
    `replaced_by_session_id` VARCHAR(36) NULL,
    `last_used_at` DATETIME(6) NULL,
    `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_refresh_sessions_token_hash` (`refresh_token_hash`),
    KEY `idx_refresh_sessions_user_active` (`user_id`, `revoked_at`, `expires_at`),
    KEY `idx_refresh_sessions_expires_at` (`expires_at`),
    KEY `idx_refresh_sessions_replaced_by` (`replaced_by_session_id`),
    CONSTRAINT `fk_refresh_sessions_user`
        FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE,
    CONSTRAINT `fk_refresh_sessions_replacement`
        FOREIGN KEY (`replaced_by_session_id`) REFERENCES `refresh_sessions` (`id`) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='可撤销的轮换刷新会话';
