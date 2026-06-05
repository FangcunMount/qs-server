-- Plan 统计拆分：
-- - activity 继续读 statistics_plan_daily（000027 已建表与索引）
-- - fulfillment 直接从 assessment_task 按 planned_at / expire_at cohort 查询
-- expire_at 侧已有 000026 的 idx_task_org_deleted_expire_status；
-- 本迁移补 planned_at cohort 的机构级/计划级窗口与趋势查询索引。

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_task_org_deleted_planned_status'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment_task` ADD INDEX `idx_task_org_deleted_planned_status` (`org_id`, `deleted_at`, `planned_at`, `status`, `plan_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
