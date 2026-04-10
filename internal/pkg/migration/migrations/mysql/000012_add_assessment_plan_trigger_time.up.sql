ALTER TABLE `assessment_plan`
    ADD COLUMN `trigger_time` VARCHAR(8) NOT NULL DEFAULT '19:00:00' COMMENT '任务触发时间（格式：HH:MM:SS）'
    AFTER `schedule_type`;
