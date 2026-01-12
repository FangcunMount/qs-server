-- 加速统计查询的联合索引
-- 查询模式：WHERE org_id = ? AND questionnaire_code = ? AND created_at >= ? AND deleted_at IS NULL
ALTER TABLE `assessment`
    ADD INDEX `idx_assessment_org_code_created_deleted` (`org_id`, `questionnaire_code`, `created_at`, `deleted_at`);

-- 查询模式：WHERE org_id = ? AND questionnaire_code = ? AND status = 'interpreted' AND created_at >= ? AND deleted_at IS NULL
ALTER TABLE `assessment`
    ADD INDEX `idx_assessment_org_code_status_created_deleted` (`org_id`, `questionnaire_code`, `status`, `created_at`, `deleted_at`);
