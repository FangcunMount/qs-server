-- 回滚时按索引逐个恢复，避免因历史库已存在同名索引而失败

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'testee'
      AND index_name = 'idx_org_id'
);
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `testee` ADD INDEX `idx_org_id` (`org_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `testee` ADD INDEX `idx_is_key_focus` (`is_key_focus`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `testee` ADD INDEX `idx_deleted_at` (`deleted_at`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_score` ADD INDEX `idx_assessment_id` (`assessment_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_score` ADD INDEX `idx_testee_id` (`testee_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_score` ADD INDEX `idx_medical_scale_id` (`medical_scale_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_score` ADD INDEX `idx_factor_code` (`factor_code`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_score` ADD INDEX `idx_deleted_at` (`deleted_at`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment` ADD INDEX `idx_org_id` (`org_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment` ADD INDEX `idx_testee_id` (`testee_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment` ADD INDEX `idx_origin_type` (`origin_type`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment` ADD INDEX `idx_origin_id` (`origin_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment` ADD INDEX `idx_status` (`status`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment` ADD INDEX `idx_medical_scale_id` (`medical_scale_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment` ADD INDEX `idx_deleted_at` (`deleted_at`)', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists = (
    SELECT COUNT(1)
    FROM information_schema.statistics
    WHERE table_schema = DATABASE()
      AND table_name = 'assessment_task'
      AND index_name = 'idx_plan_id'
);
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_task` ADD INDEX `idx_plan_id` (`plan_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_task` ADD INDEX `idx_org_id` (`org_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_task` ADD INDEX `idx_testee_id` (`testee_id`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_task` ADD INDEX `idx_expire_at` (`expire_at`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_task` ADD INDEX `idx_status` (`status`)', 'SELECT 1');
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
SET @ddl = IF(@idx_exists = 0, 'ALTER TABLE `assessment_task` ADD INDEX `idx_deleted_at` (`deleted_at`)', 'SELECT 1');
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
