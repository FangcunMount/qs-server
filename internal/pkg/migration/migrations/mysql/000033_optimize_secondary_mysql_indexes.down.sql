-- 回滚外围管理表与异步队列联合索引。

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_plan'
      AND index_name = 'idx_plan_org_deleted_scale_status_id'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `assessment_plan` DROP INDEX `idx_plan_org_deleted_scale_status_id`',
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
    @idx_exists > 0,
    'ALTER TABLE `assessment_plan` DROP INDEX `idx_plan_org_deleted_id`',
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
    @idx_exists > 0,
    'ALTER TABLE `staff` DROP INDEX `idx_staff_org_active_deleted_id`',
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
    @idx_exists > 0,
    'ALTER TABLE `staff` DROP INDEX `idx_staff_org_deleted_id`',
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
    @idx_exists > 0,
    'ALTER TABLE `clinician` DROP INDEX `idx_clinician_org_active_deleted_id`',
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
    @idx_exists > 0,
    'ALTER TABLE `clinician` DROP INDEX `idx_clinician_org_deleted_id`',
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
    @idx_exists > 0,
    'ALTER TABLE `assessment_entry` DROP INDEX `idx_assessment_entry_org_active_deleted_expire`',
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
    @idx_exists > 0,
    'ALTER TABLE `assessment_entry` DROP INDEX `idx_assessment_entry_org_deleted_id`',
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
    @idx_exists > 0,
    'ALTER TABLE `clinician_relation` DROP INDEX `idx_relation_org_clinician_deleted_bound`',
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
    @idx_exists > 0,
    'ALTER TABLE `domain_event_outbox` DROP INDEX `idx_outbox_status_created`',
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
    @idx_exists > 0,
    'ALTER TABLE `domain_event_outbox` DROP INDEX `idx_outbox_status_updated_created`',
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
    @idx_exists > 0,
    'ALTER TABLE `domain_event_outbox` DROP INDEX `idx_outbox_status_due_created`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'analytics_pending_event'
      AND index_name = 'idx_analytics_pending_event_deleted_due'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `analytics_pending_event` DROP INDEX `idx_analytics_pending_event_deleted_due`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
