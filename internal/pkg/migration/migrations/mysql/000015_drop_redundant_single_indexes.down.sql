ALTER TABLE `testee`
    ADD INDEX `idx_org_id` (`org_id`),
    ADD INDEX `idx_is_key_focus` (`is_key_focus`),
    ADD INDEX `idx_deleted_at` (`deleted_at`);

ALTER TABLE `assessment_score`
    ADD INDEX `idx_assessment_id` (`assessment_id`),
    ADD INDEX `idx_testee_id` (`testee_id`),
    ADD INDEX `idx_medical_scale_id` (`medical_scale_id`),
    ADD INDEX `idx_factor_code` (`factor_code`),
    ADD INDEX `idx_deleted_at` (`deleted_at`);

ALTER TABLE `assessment`
    ADD INDEX `idx_org_id` (`org_id`),
    ADD INDEX `idx_testee_id` (`testee_id`),
    ADD INDEX `idx_origin_type` (`origin_type`),
    ADD INDEX `idx_origin_id` (`origin_id`),
    ADD INDEX `idx_status` (`status`),
    ADD INDEX `idx_medical_scale_id` (`medical_scale_id`),
    ADD INDEX `idx_deleted_at` (`deleted_at`);

ALTER TABLE `assessment_task`
    ADD INDEX `idx_plan_id` (`plan_id`),
    ADD INDEX `idx_org_id` (`org_id`),
    ADD INDEX `idx_testee_id` (`testee_id`),
    ADD INDEX `idx_expire_at` (`expire_at`),
    ADD INDEX `idx_status` (`status`),
    ADD INDEX `idx_deleted_at` (`deleted_at`);
