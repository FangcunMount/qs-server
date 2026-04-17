ALTER TABLE `assessment`
    ADD INDEX `idx_assessment_org_code_deleted_interpreted` (`org_id`, `questionnaire_code`, `deleted_at`, `interpreted_at`),
    ADD INDEX `idx_assessment_org_deleted_interpreted` (`org_id`, `deleted_at`, `interpreted_at`);
