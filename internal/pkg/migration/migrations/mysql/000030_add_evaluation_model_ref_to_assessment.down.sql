ALTER TABLE `assessment`
    DROP INDEX `idx_assessment_evaluation_model`,
    DROP COLUMN `evaluation_model_title`,
    DROP COLUMN `evaluation_model_version`,
    DROP COLUMN `evaluation_model_code`,
    DROP COLUMN `evaluation_model_kind`;
