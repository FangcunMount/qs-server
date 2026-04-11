-- 删除已被联合索引覆盖的单列索引
-- 范围仅限：
-- - assessment_task
-- - assessment
-- - assessment_score
-- - testee
--
-- 说明：
-- - 只删除单列索引，不处理旧组合索引
-- - idx_deleted_at 虽然仍保留在共享 AuditFields tag 中，但项目不跑 AutoMigrate，
--   因此本 migration 仍可安全删除数据库中的单列索引
-- - 线上历史库可能存在 schema 漂移，因此这里按索引逐个做存在性判断，
--   避免因为单个索引缺失把整个数据库打进 dirty 状态

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_plan_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_task` DROP INDEX `idx_plan_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_org_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_task` DROP INDEX `idx_org_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_testee_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_task` DROP INDEX `idx_testee_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_expire_at'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_task` DROP INDEX `idx_expire_at`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_status'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_task` DROP INDEX `idx_status`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_deleted_at'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_task` DROP INDEX `idx_deleted_at`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_org_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment` DROP INDEX `idx_org_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_testee_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment` DROP INDEX `idx_testee_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_origin_type'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment` DROP INDEX `idx_origin_type`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_origin_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment` DROP INDEX `idx_origin_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_status'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment` DROP INDEX `idx_status`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_medical_scale_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment` DROP INDEX `idx_medical_scale_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment'
      AND index_name = 'idx_deleted_at'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment` DROP INDEX `idx_deleted_at`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_score'
      AND index_name = 'idx_assessment_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_score` DROP INDEX `idx_assessment_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_score'
      AND index_name = 'idx_testee_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_score` DROP INDEX `idx_testee_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_score'
      AND index_name = 'idx_medical_scale_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_score` DROP INDEX `idx_medical_scale_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_score'
      AND index_name = 'idx_factor_code'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_score` DROP INDEX `idx_factor_code`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_score'
      AND index_name = 'idx_deleted_at'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `assessment_score` DROP INDEX `idx_deleted_at`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'testee'
      AND index_name = 'idx_org_id'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `testee` DROP INDEX `idx_org_id`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'testee'
      AND index_name = 'idx_is_key_focus'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `testee` DROP INDEX `idx_is_key_focus`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'testee'
      AND index_name = 'idx_deleted_at'
);
SET @ddl = IF(@idx_exists > 0, 'ALTER TABLE `testee` DROP INDEX `idx_deleted_at`', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
