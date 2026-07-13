-- 013_websocket_delivery.sql
-- Cross-replica WebSocket delivery metadata and explicit client acknowledgement.

DELIMITER $$
DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`$$
CREATE PROCEDURE `AddColumnIfNotExists`(IN p_table_name VARCHAR(100), IN p_column_name VARCHAR(100), IN p_column_def TEXT)
BEGIN
    SET @col_exists = (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = p_table_name AND column_name = p_column_name);
    IF @col_exists = 0 THEN
        SET @sql = CONCAT('ALTER TABLE `', p_table_name, '` ADD COLUMN `', p_column_name, '` ', p_column_def);
        PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    END IF;
END$$
DROP PROCEDURE IF EXISTS `CreateIndexIfNotExists`$$
CREATE PROCEDURE `CreateIndexIfNotExists`(IN p_table_name VARCHAR(100), IN p_index_name VARCHAR(100), IN p_index_def TEXT, IN p_unique BOOLEAN)
BEGIN
    SET @idx_exists = (SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = p_table_name AND index_name = p_index_name);
    IF @idx_exists = 0 THEN
        SET @sql = CONCAT(IF(p_unique, 'CREATE UNIQUE INDEX `', 'CREATE INDEX `'), p_index_name, '` ON `', p_table_name, '` (', p_index_def, ')');
        PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
    END IF;
END$$
DELIMITER ;

CALL AddColumnIfNotExists('web_socket_messages', 'message_id', 'VARCHAR(64) NULL COMMENT ''全局消息ID'' AFTER `user_id`');
CALL AddColumnIfNotExists('web_socket_messages', 'sequence', 'BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT ''用户级单调序号'' AFTER `message_id`');
CALL AddColumnIfNotExists('web_socket_messages', 'expires_at', 'TIMESTAMP NULL DEFAULT NULL COMMENT ''消息过期时间'' AFTER `created_at`');
CALL AddColumnIfNotExists('web_socket_messages', 'acked_at', 'TIMESTAMP NULL DEFAULT NULL COMMENT ''客户端确认时间'' AFTER `delivered_at`');

UPDATE `web_socket_messages` SET `message_id` = CONCAT('legacy-', `id`) WHERE `message_id` IS NULL OR `message_id` = '';
CALL CreateIndexIfNotExists('web_socket_messages', 'uidx_ws_message_id', '`message_id`', TRUE);
CALL CreateIndexIfNotExists('web_socket_messages', 'idx_ws_user_sequence', '`user_id`, `sequence`', FALSE);
CALL CreateIndexIfNotExists('web_socket_messages', 'idx_ws_expires_at', '`expires_at`', FALSE);

DROP PROCEDURE IF EXISTS `AddColumnIfNotExists`;
DROP PROCEDURE IF EXISTS `CreateIndexIfNotExists`;
