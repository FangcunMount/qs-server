-- 为 ClinicianTesteeRelation / AssessmentEntry 的第一阶段查询路径补联合索引
-- 说明：
-- - 关系查询需要同时支持 clinician 维度、testee 维度与 history 维度
-- - AssessmentEntry 需要支持按 clinician 列表与计数
-- - 按索引逐个做存在性判断，避免历史库 schema 漂移导致 dirty migration

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'clinician_relation'
      AND index_name = 'idx_relation_org_clinician_active_type_deleted_bound'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `clinician_relation` ADD INDEX `idx_relation_org_clinician_active_type_deleted_bound` (`org_id`, `clinician_id`, `is_active`, `relation_type`, `deleted_at`, `bound_at`, `id`)',
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
      AND index_name = 'idx_relation_org_testee_active_type_deleted_bound'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `clinician_relation` ADD INDEX `idx_relation_org_testee_active_type_deleted_bound` (`org_id`, `testee_id`, `is_active`, `relation_type`, `deleted_at`, `bound_at`, `id`)',
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
      AND index_name = 'idx_relation_org_testee_deleted_bound'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `clinician_relation` ADD INDEX `idx_relation_org_testee_deleted_bound` (`org_id`, `testee_id`, `deleted_at`, `bound_at`, `id`)',
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
      AND index_name = 'idx_assessment_entry_org_clinician_deleted_id'
);
SET @ddl = IF(
    @idx_exists = 0,
    'ALTER TABLE `assessment_entry` ADD INDEX `idx_assessment_entry_org_clinician_deleted_id` (`org_id`, `clinician_id`, `deleted_at`, `id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
