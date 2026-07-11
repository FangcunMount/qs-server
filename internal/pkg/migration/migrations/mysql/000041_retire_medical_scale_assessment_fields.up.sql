-- Retire legacy medical_scale_* assessment storage after evaluation_model_*
-- became the sole historical model reference. The backfill is idempotent and
-- must run before dropping the obsolete compatibility columns.

UPDATE `assessment`
SET
    `evaluation_model_kind` = 'scale',
    `evaluation_model_code` = `medical_scale_code`,
    `evaluation_model_title` = `medical_scale_name`,
    `evaluation_model_algorithm` = COALESCE(`evaluation_model_algorithm`, 'scale_default')
WHERE (`evaluation_model_kind` IS NULL OR `evaluation_model_kind` = '')
  AND `medical_scale_code` IS NOT NULL
  AND `medical_scale_code` <> '';

ALTER TABLE `assessment`
    DROP INDEX `idx_assessment_testee_scale_deleted_id`,
    ADD INDEX `idx_assessment_testee_model_deleted_id` (`testee_id`, `evaluation_model_kind`, `evaluation_model_code`, `deleted_at`, `id`),
    DROP COLUMN `medical_scale_id`,
    DROP COLUMN `medical_scale_code`,
    DROP COLUMN `medical_scale_name`;

ALTER TABLE `assessment_score`
    DROP INDEX `idx_score_testee_scale_deleted_id`,
    DROP COLUMN `medical_scale_id`,
    DROP COLUMN `medical_scale_code`;
