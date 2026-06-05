-- 回滚冗余索引清理，恢复本 migration 删除的历史索引。

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'analytics_projector_checkpoint'
      AND index_name = 'idx_analytics_projector_checkpoint_deleted_at'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `analytics_projector_checkpoint` ADD INDEX `idx_analytics_projector_checkpoint_deleted_at` (`deleted_at`)',
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
    @idx_exists = 0,
    'ALTER TABLE `analytics_projector_checkpoint` ADD INDEX `idx_analytics_projector_checkpoint_status` (`status`)',
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
    @idx_exists = 0,
    'ALTER TABLE `assessment_score` ADD INDEX `idx_risk_level` (`risk_level`)',
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
    @idx_exists = 0,
    'ALTER TABLE `assessment_task` ADD INDEX `idx_assessment_id` (`assessment_id`)',
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
    @idx_exists = 0,
    'ALTER TABLE `assessment` ADD INDEX `idx_assessment_org_code_status_created_deleted` (`org_id`, `questionnaire_code`, `status`, `created_at`, `deleted_at`)',
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
      AND index_name = 'idx_assessment_org_code_created_deleted'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment` ADD INDEX `idx_assessment_org_code_created_deleted` (`org_id`, `questionnaire_code`, `created_at`, `deleted_at`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
