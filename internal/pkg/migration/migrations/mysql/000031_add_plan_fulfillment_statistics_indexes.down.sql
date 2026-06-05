SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_task_org_deleted_planned_status'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment_task` DROP INDEX `idx_task_org_deleted_planned_status`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
