-- 修复兑换中心表结构
-- 执行时间：2026-02-20

-- 1. 检查并补齐 exchange_accounts 表的兑换时间字段
ALTER TABLE `exchange_accounts`
ADD COLUMN IF NOT EXISTS `exchange_time_1` TIME DEFAULT '10:00:00' COMMENT '第一抢兑时间' AFTER `remark`,
ADD COLUMN IF NOT EXISTS `exchange_time_2` TIME DEFAULT '16:00:00' COMMENT '第二抢兑时间' AFTER `exchange_time_1`;

-- 2. 为 products 表补齐库存状态字段（如不存在）
ALTER TABLE `products`
ADD COLUMN IF NOT EXISTS `stock_status` VARCHAR(20) DEFAULT 'unknown' COMMENT '库存状态：unknown, available, sold_out' AFTER `daily_remainder_count`,
ADD COLUMN IF NOT EXISTS `last_stock_check` DATETIME NULL COMMENT '最后库存检查时间' AFTER `stock_status`;

-- 3. 确保系统配置表存在商品分类配置（字段名统一使用 key_name / key_value）
INSERT IGNORE INTO `system_configs` (`key_name`, `key_value`, `description`) VALUES
('product_categories', '其他权益奖品,视频类会员,音乐类会员,外卖美食权益,快递寄件券,云盘转存券,实用工具类,奶茶饮品权益,咖啡饮品权益,游戏礼包权益,全国通用流量权益', '商品分类列表');
