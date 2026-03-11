-- 创建用户表
CREATE TABLE IF NOT EXISTS `users` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `username` VARCHAR(50) NOT NULL UNIQUE,
    `password` VARCHAR(255) NOT NULL COMMENT 'bcrypt哈希',
    `email` VARCHAR(100),
    `role` VARCHAR(10) DEFAULT 'user' COMMENT 'user, admin',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL,
    INDEX `idx_username` (`username`),
    INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建账号表
CREATE TABLE IF NOT EXISTS `accounts` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `user_id` BIGINT UNSIGNED NOT NULL,
    `phone` VARCHAR(20) NOT NULL,
    `auth` TEXT NOT NULL COMMENT 'Basic Auth字符串',
    `token` TEXT COMMENT 'Token',
    `jwt_token` TEXT COMMENT 'JWT Token',
    `platform` VARCHAR(20) DEFAULT 'pc',
    `expire_at` BIGINT COMMENT '过期时间戳(毫秒)',
    `cloud_count` INT DEFAULT 0 COMMENT '当前云朵数量',
    `remark` VARCHAR(200) COMMENT '备注',
    `is_active` BOOLEAN DEFAULT TRUE,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL,
    FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE,
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_phone` (`phone`),
    INDEX `idx_expire_at` (`expire_at`),
    INDEX `idx_deleted_at` (`deleted_at`),
    INDEX `idx_user_active` (`user_id`, `is_active`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建任务日志表
CREATE TABLE IF NOT EXISTS `task_logs` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `user_id` BIGINT UNSIGNED NOT NULL,
    `account_id` BIGINT UNSIGNED NOT NULL,
    `task_type` VARCHAR(50) NOT NULL COMMENT 'signin, wechat, shake等',
    `status` VARCHAR(20) DEFAULT 'pending' COMMENT 'success, failed, pending',
    `message` TEXT COMMENT '执行结果/错误信息',
    `cloud_gained` INT DEFAULT 0 COMMENT '获得云朵数',
    `execution_time` INT DEFAULT 0 COMMENT '执行时长(毫秒)',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL,
    FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE,
    FOREIGN KEY (`account_id`) REFERENCES `accounts`(`id`) ON DELETE CASCADE,
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_account_id` (`account_id`),
    INDEX `idx_created_at` (`created_at`),
    INDEX `idx_deleted_at` (`deleted_at`),
    INDEX `idx_task_status` (`task_type`, `status`),
    INDEX `idx_account_created` (`account_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建云朵统计表
CREATE TABLE IF NOT EXISTS `cloud_stats` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `user_id` BIGINT UNSIGNED NOT NULL,
    `account_id` BIGINT UNSIGNED NOT NULL,
    `date` DATE NOT NULL COMMENT '统计日期',
    `cloud_count` INT NOT NULL COMMENT '当日云朵数',
    `cloud_diff` INT DEFAULT 0 COMMENT '对比昨日变化',
    `cloud_diff_week` INT DEFAULT 0 COMMENT '对比上周变化',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL,
    FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE,
    FOREIGN KEY (`account_id`) REFERENCES `accounts`(`id`) ON DELETE CASCADE,
    UNIQUE KEY `uk_user_account_date` (`user_id`, `account_id`, `date`),
    INDEX `idx_date` (`date`),
    INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建系统配置表
CREATE TABLE IF NOT EXISTS `system_configs` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `key_name` VARCHAR(100) NOT NULL UNIQUE,
    `key_value` TEXT NOT NULL,
    `description` VARCHAR(200),
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_key_name` (`key_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 插入默认系统配置（使用 INSERT IGNORE 兼容 MySQL 5.7 和 8.0 全版本）
INSERT IGNORE INTO `system_configs` (`key_name`, `key_value`, `description`) VALUES
('task_concurrency', '10', '任务并发数量'),
('task_schedule', '0 8 * * *', '定时任务执行时间(Cron表达式)'),
('token_cache_ttl', '86400', 'Token缓存时间(秒)');

-- 创建任务配置表（管理员控制任务上下架）
CREATE TABLE IF NOT EXISTS `task_configs` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `task_type` VARCHAR(50) NOT NULL UNIQUE COMMENT '任务类型标识',
    `task_name` VARCHAR(50) NOT NULL COMMENT '任务名称',
    `is_enabled` BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    `sort_order` INT DEFAULT 0 COMMENT '排序',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL,
    INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO `task_configs` (`task_type`, `task_name`, `is_enabled`, `sort_order`) VALUES
('signin', '签到', TRUE, 1),
('tasklist', '任务列表', TRUE, 2),
('wechat', '微信签到', TRUE, 3),
('wxdraw', '微信抽奖', TRUE, 4),
('todaycloud', '今日云朵', TRUE, 5),
('invitefriends', '邀请好友', TRUE, 6),
('shake', '摇一摇', TRUE, 7),
('receive', '领取云朵', TRUE, 8),
('messagepush', '消息推送', TRUE, 9),
('backupgift', '备份礼包', TRUE, 10),
('blindbox', '盲盒', TRUE, 11),
('redpacket', '红包', TRUE, 12),
('aicloud', 'AI云朵', TRUE, 13),
('garden', '花园', TRUE, 14),
('cloudphone', '云手机红包', TRUE, 15),
('cloudbattle', '云朵大战', TRUE, 16),
('exchange', '兑换月卡', TRUE, 17);
