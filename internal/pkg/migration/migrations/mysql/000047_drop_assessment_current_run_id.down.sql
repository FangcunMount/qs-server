ALTER TABLE `assessment`
    ADD COLUMN `current_run_id` varchar(100) DEFAULT NULL COMMENT '当前执行运行ID' AFTER `failure_reason`,
    ADD INDEX `idx_assessment_current_run_id` (`current_run_id`);
