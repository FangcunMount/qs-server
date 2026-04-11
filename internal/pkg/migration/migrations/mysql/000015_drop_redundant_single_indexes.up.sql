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

ALTER TABLE `assessment_task`
    DROP INDEX `idx_plan_id`,
    DROP INDEX `idx_org_id`,
    DROP INDEX `idx_testee_id`,
    DROP INDEX `idx_expire_at`,
    DROP INDEX `idx_status`,
    DROP INDEX `idx_deleted_at`;

ALTER TABLE `assessment`
    DROP INDEX `idx_org_id`,
    DROP INDEX `idx_testee_id`,
    DROP INDEX `idx_origin_type`,
    DROP INDEX `idx_origin_id`,
    DROP INDEX `idx_status`,
    DROP INDEX `idx_medical_scale_id`,
    DROP INDEX `idx_deleted_at`;

ALTER TABLE `assessment_score`
    DROP INDEX `idx_assessment_id`,
    DROP INDEX `idx_testee_id`,
    DROP INDEX `idx_medical_scale_id`,
    DROP INDEX `idx_factor_code`,
    DROP INDEX `idx_deleted_at`;

ALTER TABLE `testee`
    DROP INDEX `idx_org_id`,
    DROP INDEX `idx_is_key_focus`,
    DROP INDEX `idx_deleted_at`;
