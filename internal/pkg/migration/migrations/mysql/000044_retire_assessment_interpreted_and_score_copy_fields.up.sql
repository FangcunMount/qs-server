-- Assessment 的领域终态收敛为 evaluated；报告完成状态由 Interpretation 管理。
-- 先保留可追溯的评分完成时间，再移除旧 interpreted 兼容字段。
ALTER TABLE `assessment`
  ADD COLUMN `evaluated_at` datetime DEFAULT NULL COMMENT '评分事实可靠提交时间' AFTER `submitted_at`;

UPDATE `assessment`
SET `status` = 'evaluated',
    `evaluated_at` = COALESCE(`interpreted_at`, `updated_at`, `created_at`)
WHERE `status` = 'interpreted';

UPDATE `assessment`
SET `evaluated_at` = COALESCE(`evaluated_at`, `updated_at`, `created_at`)
WHERE `status` = 'evaluated' AND `evaluated_at` IS NULL;

ALTER TABLE `assessment`
  DROP COLUMN `interpreted_at`;

ALTER TABLE `assessment_score`
  DROP COLUMN `conclusion`,
  DROP COLUMN `suggestion`;
