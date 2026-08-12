-- 019_websocket_sequence_allocator.sql
-- MySQL owns the per-user replay cursor. Redis Pub/Sub remains transport-only.

CREATE TABLE IF NOT EXISTS `web_socket_sequences` (
    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户ID',
    `sequence` BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '已分配的最大事件序号',
    `updated_at` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '最后分配时间',
    PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='WebSocket用户级事件序号分配器';

-- Preserve compatibility with envelopes created before the allocator existed.
-- New allocations use INSERT ... ON DUPLICATE KEY UPDATE under the row lock.
INSERT INTO `web_socket_sequences` (`user_id`, `sequence`)
SELECT `user_id`, COALESCE(MAX(`sequence`), 0)
FROM `web_socket_messages`
GROUP BY `user_id`
ON DUPLICATE KEY UPDATE `sequence` = GREATEST(`sequence`, VALUES(`sequence`));
