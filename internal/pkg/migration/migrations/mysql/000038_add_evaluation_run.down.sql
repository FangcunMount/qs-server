ALTER TABLE `assessment`
    DROP INDEX `idx_assessment_current_run_id`,
    DROP COLUMN `current_run_id`;

DROP TABLE IF EXISTS `evaluation_run`;
