-- 为外围管理表与异步队列补充查询形态联合索引。
-- 说明：
-- - 只新增索引，不删除历史索引，先降低慢查询风险
-- - 按索引逐个做存在性判断，避免历史库 schema 漂移导致 dirty migration

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'analytics_pending_event'
      AND index_name = 'idx_analytics_pending_event_deleted_due'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `analytics_pending_event` ADD INDEX `idx_analytics_pending_event_deleted_due` (`deleted_at`, `next_attempt_at`, `event_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'domain_event_outbox'
      AND index_name = 'idx_outbox_status_due_created'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `domain_event_outbox` ADD INDEX `idx_outbox_status_due_created` (`status`, `next_attempt_at`, `created_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'domain_event_outbox'
      AND index_name = 'idx_outbox_status_updated_created'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `domain_event_outbox` ADD INDEX `idx_outbox_status_updated_created` (`status`, `updated_at`, `created_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'domain_event_outbox'
      AND index_name = 'idx_outbox_status_created'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `domain_event_outbox` ADD INDEX `idx_outbox_status_created` (`status`, `created_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'clinician_relation'
      AND index_name = 'idx_relation_org_clinician_deleted_bound'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `clinician_relation` ADD INDEX `idx_relation_org_clinician_deleted_bound` (`org_id`, `clinician_id`, `deleted_at`, `bound_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_entry'
      AND index_name = 'idx_assessment_entry_org_deleted_id'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment_entry` ADD INDEX `idx_assessment_entry_org_deleted_id` (`org_id`, `deleted_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_entry'
      AND index_name = 'idx_assessment_entry_org_active_deleted_expire'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment_entry` ADD INDEX `idx_assessment_entry_org_active_deleted_expire` (`org_id`, `is_active`, `deleted_at`, `expires_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'clinician'
      AND index_name = 'idx_clinician_org_deleted_id'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `clinician` ADD INDEX `idx_clinician_org_deleted_id` (`org_id`, `deleted_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'clinician'
      AND index_name = 'idx_clinician_org_active_deleted_id'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `clinician` ADD INDEX `idx_clinician_org_active_deleted_id` (`org_id`, `is_active`, `deleted_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'staff'
      AND index_name = 'idx_staff_org_deleted_id'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `staff` ADD INDEX `idx_staff_org_deleted_id` (`org_id`, `deleted_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'staff'
      AND index_name = 'idx_staff_org_active_deleted_id'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `staff` ADD INDEX `idx_staff_org_active_deleted_id` (`org_id`, `is_active`, `deleted_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_plan'
      AND index_name = 'idx_plan_org_deleted_id'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment_plan` ADD INDEX `idx_plan_org_deleted_id` (`org_id`, `deleted_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_plan'
      AND index_name = 'idx_plan_org_deleted_scale_status_id'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment_plan` ADD INDEX `idx_plan_org_deleted_scale_status_id` (`org_id`, `deleted_at`, `scale_code`, `status`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
