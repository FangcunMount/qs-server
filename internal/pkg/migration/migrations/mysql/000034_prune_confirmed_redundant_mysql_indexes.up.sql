-- 删除已经由更准确联合索引或唯一索引覆盖的冗余索引。
-- 说明：
-- - 只处理确定性高的候选
-- - 按索引逐个做存在性判断，避免历史库 schema 漂移导致 dirty migration

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_assessment_org_code_created_deleted'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment` DROP INDEX `idx_assessment_org_code_created_deleted`',
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
      AND index_name = 'idx_assessment_org_code_status_created_deleted'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment` DROP INDEX `idx_assessment_org_code_status_created_deleted`',
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
      AND index_name = 'idx_assessment_id'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment_task` DROP INDEX `idx_assessment_id`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_score'
      AND index_name = 'idx_risk_level'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment_score` DROP INDEX `idx_risk_level`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'analytics_projector_checkpoint'
      AND index_name = 'idx_analytics_projector_checkpoint_status'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `analytics_projector_checkpoint` DROP INDEX `idx_analytics_projector_checkpoint_status`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'analytics_projector_checkpoint'
      AND index_name = 'idx_analytics_projector_checkpoint_deleted_at'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `analytics_projector_checkpoint` DROP INDEX `idx_analytics_projector_checkpoint_deleted_at`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
