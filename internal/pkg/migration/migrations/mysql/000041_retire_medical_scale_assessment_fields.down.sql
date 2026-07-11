-- Schema-only rollback. Legacy numeric scale IDs cannot be reconstructed from
-- canonical model identity and are intentionally not reintroduced into code.

ALTER TABLE `assessment`
    ADD COLUMN `medical_scale_id` bigint unsigned DEFAULT NULL COMMENT 'legacy scale id',
    ADD COLUMN `medical_scale_code` varchar(100) DEFAULT NULL COMMENT 'legacy scale code',
    ADD COLUMN `medical_scale_name` varchar(255) DEFAULT NULL COMMENT 'legacy scale name',
    ADD INDEX `idx_assessment_testee_scale_deleted_id` (`testee_id`, `medical_scale_id`, `deleted_at`, `id`);

UPDATE `assessment`
SET
    `medical_scale_code` = `evaluation_model_code`,
    `medical_scale_name` = `evaluation_model_title`
WHERE `evaluation_model_kind` = 'scale';

ALTER TABLE `assessment_score`
    ADD COLUMN `medical_scale_id` bigint unsigned DEFAULT NULL COMMENT 'legacy scale id',
    ADD COLUMN `medical_scale_code` varchar(100) DEFAULT NULL COMMENT 'legacy scale code',
    ADD INDEX `idx_score_testee_scale_deleted_id` (`testee_id`, `medical_scale_id`, `deleted_at`, `id`);
