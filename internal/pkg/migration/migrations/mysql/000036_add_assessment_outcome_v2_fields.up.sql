-- Assessment outcome v2 projection columns.
-- Check before backfill:
--   SELECT COUNT(*) FROM assessment WHERE status = 'interpreted' AND total_score IS NOT NULL;
--   SELECT evaluation_model_kind, COUNT(*) FROM assessment GROUP BY evaluation_model_kind;

ALTER TABLE `assessment`
    ADD COLUMN `evaluation_model_sub_kind` varchar(50) DEFAULT NULL COMMENT '解释模型子类型：trait/typology',
    ADD COLUMN `evaluation_model_algorithm` varchar(50) DEFAULT NULL COMMENT '解释模型算法：scale_default/mbti/sbti',
    ADD COLUMN `primary_score_kind` varchar(50) DEFAULT NULL COMMENT '主分类型：raw_total/match_percent',
    ADD COLUMN `primary_score_value` double DEFAULT NULL COMMENT '主分数值',
    ADD COLUMN `primary_score_label` varchar(100) DEFAULT NULL COMMENT '主分展示标签',
    ADD COLUMN `primary_score_max` double DEFAULT NULL COMMENT '主分满分',
    ADD COLUMN `level_code` varchar(50) DEFAULT NULL COMMENT '结果等级编码',
    ADD COLUMN `level_label` varchar(100) DEFAULT NULL COMMENT '结果等级展示标签',
    ADD COLUMN `severity` varchar(50) DEFAULT NULL COMMENT '严重度：none/low/medium/high',
    ADD INDEX `idx_assessment_severity` (`severity`),
    ADD INDEX `idx_assessment_level_code` (`level_code`);

UPDATE `assessment`
SET
    `evaluation_model_algorithm` = CASE `evaluation_model_kind`
        WHEN 'scale' THEN 'scale_default'
        WHEN 'mbti' THEN 'mbti'
        WHEN 'sbti' THEN 'sbti'
        ELSE NULL
    END,
    `evaluation_model_sub_kind` = CASE `evaluation_model_kind`
        WHEN 'mbti' THEN 'typology'
        WHEN 'sbti' THEN 'typology'
        ELSE ''
    END,
    `primary_score_kind` = CASE
        WHEN `evaluation_model_kind` IN ('mbti', 'sbti') THEN 'match_percent'
        WHEN `total_score` IS NOT NULL THEN 'raw_total'
        ELSE NULL
    END,
    `primary_score_value` = `total_score`,
    `level_code` = COALESCE(NULLIF(`risk_level`, ''), NULL),
    `level_label` = COALESCE(NULLIF(`risk_level`, ''), NULL),
    `severity` = CASE `risk_level`
        WHEN 'severe' THEN 'high'
        WHEN 'high' THEN 'high'
        WHEN 'medium' THEN 'medium'
        WHEN 'low' THEN 'low'
        WHEN 'none' THEN 'none'
        ELSE NULL
    END
WHERE `status` = 'interpreted';
