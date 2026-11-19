-- Store confirmed music-note earnings separately from cloud rewards.
DELIMITER $$

DROP PROCEDURE IF EXISTS `AddMiguNoteRewardColumn`$$
CREATE PROCEDURE `AddMiguNoteRewardColumn`(IN p_table_name VARCHAR(100))
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = p_table_name
          AND column_name = 'migu_notes_gained'
    ) THEN
        SET @sql = CONCAT(
            'ALTER TABLE `', p_table_name,
            '` ADD COLUMN `migu_notes_gained` BIGINT NOT NULL DEFAULT 0 COMMENT ''已确认的咪咕AI音符收益'''
        );
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

CALL AddMiguNoteRewardColumn('task_logs');
CALL AddMiguNoteRewardColumn('task_logs_archive');
DROP PROCEDURE IF EXISTS `AddMiguNoteRewardColumn`;

-- The old song-task message records only amounts returned for newly claimed
-- tiers whose received status was verified. Preserve those receipts on upgrade.
-- Wallet balances and unread reward notifications are deliberately excluded.
UPDATE `task_logs`
SET `migu_notes_gained` = CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(`message`, '本次AI音符', -1), '，', 1) AS UNSIGNED)
WHERE `task_type` = 'migu_song'
  AND `migu_notes_gained` = 0
  AND `message` REGEXP '^咪咕听歌[0-9]+[.][0-9]+分钟，已确认领取[0-9]+/[0-9]+档，本次AI音符[0-9]{1,9}(，其余档位待确认（未重复领取）)?$';

UPDATE `task_logs_archive`
SET `migu_notes_gained` = CAST(SUBSTRING_INDEX(SUBSTRING_INDEX(`message`, '本次AI音符', -1), '，', 1) AS UNSIGNED)
WHERE `task_type` = 'migu_song'
  AND `migu_notes_gained` = 0
  AND `message` REGEXP '^咪咕听歌[0-9]+[.][0-9]+分钟，已确认领取[0-9]+/[0-9]+档，本次AI音符[0-9]{1,9}(，其余档位待确认（未重复领取）)?$';
