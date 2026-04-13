-- 回滚 ClinicianTesteeRelation / AssessmentEntry 第一阶段联合索引

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'clinician_relation'
      AND index_name = 'idx_relation_org_clinician_active_type_deleted_bound'
);
SET @ddl = IF(
    @idx_exists > 0,
    'ALTER TABLE `clinician_relation` DROP INDEX `idx_relation_org_clinician_active_type_deleted_bound`',
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
    @idx_exists > 0,
    'ALTER TABLE `clinician_relation` DROP INDEX `idx_relation_org_testee_active_type_deleted_bound`',
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
    @idx_exists > 0,
    'ALTER TABLE `clinician_relation` DROP INDEX `idx_relation_org_testee_deleted_bound`',
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
    @idx_exists > 0,
    'ALTER TABLE `assessment_entry` DROP INDEX `idx_assessment_entry_org_clinician_deleted_id`',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
