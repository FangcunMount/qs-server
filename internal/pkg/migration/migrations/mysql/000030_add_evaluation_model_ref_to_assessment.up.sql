-- 为 Evaluation Engine 引入通用解释模型引用。
-- 本迁移只新增 nullable 字段并从旧 medical_scale_* 字段回填，不删除旧字段。

ALTER TABLE `assessment`
    ADD COLUMN `evaluation_model_kind` varchar(50) DEFAULT NULL COMMENT '解释模型类型：scale/mbti/ai_profile',
    ADD COLUMN `evaluation_model_code` varchar(100) DEFAULT NULL COMMENT '解释模型编码',
    ADD COLUMN `evaluation_model_version` varchar(50) DEFAULT NULL COMMENT '解释模型版本',
    ADD COLUMN `evaluation_model_title` varchar(255) DEFAULT NULL COMMENT '解释模型标题',
    ADD INDEX `idx_assessment_evaluation_model` (`evaluation_model_kind`, `evaluation_model_code`);

UPDATE `assessment`
SET
    `evaluation_model_kind` = 'scale',
    `evaluation_model_code` = `medical_scale_code`,
    `evaluation_model_title` = `medical_scale_name`
WHERE `evaluation_model_kind` IS NULL
  AND `medical_scale_code` IS NOT NULL
  AND `medical_scale_code` <> '';
