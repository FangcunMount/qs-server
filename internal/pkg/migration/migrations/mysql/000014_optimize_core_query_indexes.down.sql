ALTER TABLE `testee`
    DROP INDEX `idx_testee_org_deleted_id`,
    DROP INDEX `idx_testee_org_focus_deleted_id`,
    DROP INDEX `idx_testee_org_deleted_created`,
    DROP INDEX `idx_testee_org_profile_deleted`;

ALTER TABLE `assessment_score`
    DROP INDEX `idx_score_assessment_deleted_total_factor`,
    DROP INDEX `idx_score_testee_factor_deleted_id`,
    DROP INDEX `idx_score_testee_scale_deleted_id`;

ALTER TABLE `assessment`
    DROP INDEX `idx_assessment_org_deleted_created`,
    DROP INDEX `idx_assessment_org_status_deleted_id`,
    DROP INDEX `idx_assessment_testee_deleted_id`,
    DROP INDEX `idx_assessment_testee_scale_deleted_id`,
    DROP INDEX `idx_assessment_origin_deleted_id`,
    DROP INDEX `idx_assessment_status_deleted_id`,
    DROP INDEX `idx_assessment_org_code_deleted_created`,
    DROP INDEX `idx_assessment_org_code_status_deleted_created`;

ALTER TABLE `assessment_task`
    DROP INDEX `idx_task_schedule_due`,
    DROP INDEX `idx_task_expire_scan`,
    DROP INDEX `idx_task_plan_deleted_seq`,
    DROP INDEX `idx_task_testee_deleted_planned`;
