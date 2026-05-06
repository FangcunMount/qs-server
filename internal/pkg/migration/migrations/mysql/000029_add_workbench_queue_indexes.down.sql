-- 回滚工作台动态队列索引。

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_assessment_workbench_risk_candidate'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment` DROP INDEX `idx_assessment_workbench_risk_candidate`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_assessment_workbench_latest_by_testee'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment` DROP INDEX `idx_assessment_workbench_latest_by_testee`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_task_workbench_followup_opened'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment_task` DROP INDEX `idx_task_workbench_followup_opened`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'testee'
      AND index_name = 'idx_testee_workbench_key_focus_order'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `testee` DROP INDEX `idx_testee_workbench_key_focus_order`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
