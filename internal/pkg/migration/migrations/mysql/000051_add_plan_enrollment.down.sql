ALTER TABLE `assessment_task`
  DROP INDEX `idx_task_org_terminal_time`,
  DROP INDEX `idx_task_enrollment_status`,
  DROP INDEX `uk_enrollment_seq`,
  ADD UNIQUE KEY `uk_plan_testee_seq` (`plan_id`, `testee_id`, `seq`),
  DROP COLUMN `canceled_at`,
  DROP COLUMN `expired_at`,
  DROP COLUMN `enrollment_id`;

DROP TABLE IF EXISTS `plan_enrollment`;
