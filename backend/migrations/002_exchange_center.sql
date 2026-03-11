-- 创建商品表 (存储商品信息)
CREATE TABLE IF NOT EXISTS `products` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `prize_id` VARCHAR(100) NOT NULL UNIQUE COMMENT '商品 ID(来自 API)',
    `prize_name` VARCHAR(255) NOT NULL COMMENT '商品名称',
    `p_order` INT DEFAULT 0 COMMENT '云朵价格',
    `category` VARCHAR(100) DEFAULT '未知分类' COMMENT '商品分类',
    `daily_remainder_count` INT DEFAULT 0 COMMENT '每日剩余数量',
    `memo` TEXT COMMENT '商品备注信息',
    `is_active` BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL,
    INDEX `idx_prize_id` (`prize_id`),
    INDEX `idx_category` (`category`),
    INDEX `idx_active` (`is_active`),
    INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建用户兑换账号表 (用户的抢兑账号)
CREATE TABLE IF NOT EXISTS `exchange_accounts` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户 ID',
    `account_id` BIGINT UNSIGNED NOT NULL COMMENT '云盘账号 ID',
    `phone` VARCHAR(20) NOT NULL COMMENT '手机号',
    `auth` TEXT NOT NULL COMMENT 'Basic Auth 字符串',
    `token` TEXT COMMENT 'Token',
    `jwt_token` TEXT COMMENT 'JWT Token',
    `remark` VARCHAR(200) COMMENT '备注',
    `exchange_time_1` TIME DEFAULT '10:00:00' COMMENT '第一次抢兑时间',
    `exchange_time_2` TIME DEFAULT '16:00:00' COMMENT '第二次抢兑时间',
    `is_active` BOOLEAN DEFAULT TRUE COMMENT '是否启用',
    `last_exchange_at` TIMESTAMP NULL COMMENT '最后抢兑时间',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL,
    FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE,
    FOREIGN KEY (`account_id`) REFERENCES `accounts`(`id`) ON DELETE CASCADE,
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_account_id` (`account_id`),
    INDEX `idx_active` (`is_active`),
    INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建抢兑任务表 (用户抢兑任务配置)
CREATE TABLE IF NOT EXISTS `exchange_tasks` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户 ID',
    `exchange_account_id` BIGINT UNSIGNED NOT NULL COMMENT '兑换账号 ID',
    `product_id` BIGINT UNSIGNED NOT NULL COMMENT '商品 ID',
    `prize_id` VARCHAR(100) NOT NULL COMMENT '商品 ID(来自 API)',
    `prize_name` VARCHAR(255) NOT NULL COMMENT '商品名称',
    `task_type` ENUM('fixed', 'long_term') DEFAULT 'fixed' COMMENT '任务类型：fixed=固定次数，long_term=长期抢兑',
    `max_attempts` INT DEFAULT 1 COMMENT '最大抢兑次数 (固定次数模式)',
    `attempted_count` INT DEFAULT 0 COMMENT '已抢兑次数',
    `status` ENUM('pending', 'running', 'completed', 'cancelled', 'failed') DEFAULT 'pending' COMMENT '任务状态',
    `last_attempt_at` TIMESTAMP NULL COMMENT '最后抢兑时间',
    `last_result` TEXT COMMENT '最后抢兑结果',
    `success_count` INT DEFAULT 0 COMMENT '成功次数',
    `fail_count` INT DEFAULT 0 COMMENT '失败次数',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    `deleted_at` TIMESTAMP NULL,
    FOREIGN KEY (`user_id`) REFERENCES `users`(`id`) ON DELETE CASCADE,
    FOREIGN KEY (`exchange_account_id`) REFERENCES `exchange_accounts`(`id`) ON DELETE CASCADE,
    FOREIGN KEY (`product_id`) REFERENCES `products`(`id`) ON DELETE CASCADE,
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_account_id` (`exchange_account_id`),
    INDEX `idx_status` (`status`),
    INDEX `idx_task_type` (`task_type`),
    INDEX `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 创建抢兑记录表 (历史抢兑记录)
CREATE TABLE IF NOT EXISTS `exchange_records` (
    `id` BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户 ID',
    `exchange_account_id` BIGINT UNSIGNED NOT NULL COMMENT '兑换账号 ID',
    `exchange_task_id` BIGINT UNSIGNED COMMENT '抢兑任务 ID',
    `product_id` BIGINT UNSIGNED NOT NULL COMMENT '商品 ID',
    `prize_id` VARCHAR(100) NOT NULL COMMENT '商品 ID(来自 API)',
    `prize_name` VARCHAR(255) NOT NULL COMMENT '商品名称',
    `status` ENUM('success', 'failed') NOT NULL COMMENT '抢兑结果',
    `message` TEXT COMMENT '抢兑结果消息',
    `execution_time_ms` INT DEFAULT 0 COMMENT '执行时长 (毫秒)',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_account_id` (`exchange_account_id`),
    INDEX `idx_task_id` (`exchange_task_id`),
    INDEX `idx_created_at` (`created_at`),
    INDEX `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 更新系统配置表添加抢兑相关配置
INSERT IGNORE INTO `system_configs` (`key_name`, `key_value`, `description`) VALUES
('exchange_concurrency', '10', '抢兑任务并发数量'),
('exchange_schedule_time_1', '10:00', '第一次抢兑时间'),
('exchange_schedule_time_2', '16:00', '第二次抢兑时间'),
('exchange_auto_update_products', 'true', '是否自动更新商品库'),
('exchange_update_time', '08:00', '商品库自动更新时间');

-- 插入默认商品分类数据
INSERT IGNORE INTO `system_configs` (`key_name`, `key_value`, `description`) VALUES
('product_categories', '其他权益奖品，视频类会员，音乐类会员，外卖美食权益，快递寄件券，云盘转存券，实用工具类，奶茶饮品权益，咖啡饮品权益，游戏礼包权益，全国通用流量权益', '商品分类列表');
