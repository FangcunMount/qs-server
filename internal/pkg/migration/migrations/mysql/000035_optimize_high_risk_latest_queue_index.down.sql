-- 回滚 high_risk 工作台最新测评队列覆盖索引。

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_assessment_workbench_latest_id_risk_by_testee'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment` DROP INDEX `idx_assessment_workbench_latest_id_risk_by_testee`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
