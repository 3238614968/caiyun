-- 005_calendar_dates.sql
-- 真实节假日/调休工作日表。调度层优先读取 calendar_dates，未覆盖日期才回退周末判断。

CREATE TABLE IF NOT EXISTS `calendar_dates` (
    `date` DATE NOT NULL COMMENT '日期',
    `day_type` VARCHAR(20) NOT NULL COMMENT 'holiday=节假日/休息日, workday=调休工作日',
    `name` VARCHAR(100) NOT NULL DEFAULT '' COMMENT '节假日/调休名称',
    `source` VARCHAR(120) NOT NULL DEFAULT '' COMMENT '数据来源',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`date`),
    INDEX `idx_calendar_dates_day_type` (`day_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='节假日与调休工作日表';

-- 内置少量 2026 法定节日日期兜底；完整连休/调休请用 scripts/import-calendar.sh 导入官方安排 CSV。
INSERT IGNORE INTO `calendar_dates` (`date`, `day_type`, `name`, `source`) VALUES
('2026-01-01', 'holiday', '元旦', 'builtin_statutory_2026'),
('2026-02-16', 'holiday', '春节除夕', 'builtin_statutory_2026'),
('2026-02-17', 'holiday', '春节', 'builtin_statutory_2026'),
('2026-02-18', 'holiday', '春节', 'builtin_statutory_2026'),
('2026-02-19', 'holiday', '春节', 'builtin_statutory_2026'),
('2026-04-05', 'holiday', '清明节', 'builtin_statutory_2026'),
('2026-05-01', 'holiday', '劳动节', 'builtin_statutory_2026'),
('2026-05-02', 'holiday', '劳动节', 'builtin_statutory_2026'),
('2026-06-19', 'holiday', '端午节', 'builtin_statutory_2026'),
('2026-09-25', 'holiday', '中秋节', 'builtin_statutory_2026'),
('2026-10-01', 'holiday', '国庆节', 'builtin_statutory_2026'),
('2026-10-02', 'holiday', '国庆节', 'builtin_statutory_2026'),
('2026-10-03', 'holiday', '国庆节', 'builtin_statutory_2026');
