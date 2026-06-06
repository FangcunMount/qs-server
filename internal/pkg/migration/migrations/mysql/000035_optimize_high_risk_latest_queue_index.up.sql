-- 为 high_risk 工作台最新测评队列补覆盖索引。
-- 说明：
-- - 查询先按 testee_id 聚合 MAX(id)，再回表过滤最新风险等级
-- - risk_level 放在尾部用于覆盖内层非空风险判断，避免聚合扫描大量回表

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_assessment_workbench_latest_id_risk_by_testee'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment` ADD INDEX `idx_assessment_workbench_latest_id_risk_by_testee` (`org_id`, `status`, `deleted_at`, `testee_id`, `id`, `risk_level`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
