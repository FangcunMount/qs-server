-- 工作台 high_risk 队列改为 MAX(id) + GROUP BY testee_id 去重；
-- 补 (org_id, status, deleted_at, testee_id, id) 覆盖索引，避免窗口函数全表排序。

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_assessment_workbench_latest_id_by_testee'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment` ADD INDEX `idx_assessment_workbench_latest_id_by_testee` (`org_id`, `status`, `deleted_at`, `testee_id`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
