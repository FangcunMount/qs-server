-- The removed interpretation timestamp and score-copy text cannot be reconstructed.
ALTER TABLE `assessment`
  ADD COLUMN `interpreted_at` datetime DEFAULT NULL COMMENT '历史兼容字段' AFTER `submitted_at`;

ALTER TABLE `assessment`
  DROP COLUMN `evaluated_at`;

ALTER TABLE `assessment_score`
  ADD COLUMN `conclusion` text COMMENT '历史兼容字段' AFTER `risk_level`,
  ADD COLUMN `suggestion` text COMMENT '历史兼容字段' AFTER `conclusion`;
