-- 为工作台动态队列补查询索引。
-- 慢路径来自全院 high_risk 最新风险判断、follow_up opened 任务去重、key_focus 列表排序。

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_assessment_workbench_risk_candidate'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment` ADD INDEX `idx_assessment_workbench_risk_candidate` (`org_id`, `status`, `deleted_at`, `risk_level`, `testee_id`, `interpreted_at`, `updated_at`, `created_at`, `id`)',
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
    @idx_exists = 0,
    'ALTER TABLE `assessment` ADD INDEX `idx_assessment_workbench_latest_by_testee` (`org_id`, `testee_id`, `status`, `deleted_at`, `interpreted_at`, `updated_at`, `created_at`, `id`, `risk_level`)',
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
    @idx_exists = 0,
    'ALTER TABLE `assessment_task` ADD INDEX `idx_task_workbench_followup_opened` (`org_id`, `status`, `deleted_at`, `testee_id`, `expire_at`, `planned_at`, `id`)',
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
    @idx_exists = 0,
    'ALTER TABLE `testee` ADD INDEX `idx_testee_workbench_key_focus_order` (`org_id`, `is_key_focus`, `deleted_at`, `created_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
