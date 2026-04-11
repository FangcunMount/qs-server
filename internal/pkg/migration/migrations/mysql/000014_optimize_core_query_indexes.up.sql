-- 第一批核心查询索引优化
-- 目标：
-- 1. 覆盖 assessment_task 的调度、过期扫描、计划/受试者列表热路径
-- 2. 覆盖 assessment 的问卷统计、系统统计、受试者/来源查询热路径
-- 3. 覆盖 assessment_score 的测评详情与趋势分析热路径
-- 4. 覆盖 testee 的机构列表、重点关注、今日新增热路径
--
-- 说明：
-- - 本批次只新增联合索引，不删除旧单列索引
-- - 删除旧索引将在慢查询/EXPLAIN 验证后单独做一批

ALTER TABLE `assessment_task`
    ADD INDEX `idx_task_schedule_due` (`org_id`, `status`, `deleted_at`, `planned_at`, `id`),
    ADD INDEX `idx_task_expire_scan` (`status`, `deleted_at`, `expire_at`, `id`),
    ADD INDEX `idx_task_plan_deleted_seq` (`plan_id`, `deleted_at`, `seq`),
    ADD INDEX `idx_task_testee_deleted_planned` (`testee_id`, `deleted_at`, `planned_at`, `plan_id`, `id`);

ALTER TABLE `assessment`
    ADD INDEX `idx_assessment_org_deleted_created` (`org_id`, `deleted_at`, `created_at`),
    ADD INDEX `idx_assessment_org_status_deleted_id` (`org_id`, `status`, `deleted_at`, `id`),
    ADD INDEX `idx_assessment_testee_deleted_id` (`testee_id`, `deleted_at`, `id`),
    ADD INDEX `idx_assessment_testee_scale_deleted_id` (`testee_id`, `medical_scale_id`, `deleted_at`, `id`),
    ADD INDEX `idx_assessment_origin_deleted_id` (`origin_type`, `origin_id`, `deleted_at`, `id`),
    ADD INDEX `idx_assessment_status_deleted_id` (`status`, `deleted_at`, `id`),
    ADD INDEX `idx_assessment_org_code_deleted_created` (`org_id`, `questionnaire_code`, `deleted_at`, `created_at`),
    ADD INDEX `idx_assessment_org_code_status_deleted_created` (`org_id`, `questionnaire_code`, `status`, `deleted_at`, `created_at`);

ALTER TABLE `assessment_score`
    ADD INDEX `idx_score_assessment_deleted_total_factor` (`assessment_id`, `deleted_at`, `is_total_score`, `factor_code`),
    ADD INDEX `idx_score_testee_factor_deleted_id` (`testee_id`, `factor_code`, `deleted_at`, `id`),
    ADD INDEX `idx_score_testee_scale_deleted_id` (`testee_id`, `medical_scale_id`, `deleted_at`, `id`);

ALTER TABLE `testee`
    ADD INDEX `idx_testee_org_deleted_id` (`org_id`, `deleted_at`, `id`),
    ADD INDEX `idx_testee_org_focus_deleted_id` (`org_id`, `is_key_focus`, `deleted_at`, `id`),
    ADD INDEX `idx_testee_org_deleted_created` (`org_id`, `deleted_at`, `created_at`),
    ADD INDEX `idx_testee_org_profile_deleted` (`org_id`, `profile_id`, `deleted_at`, `id`);
