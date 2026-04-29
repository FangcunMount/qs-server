ALTER TABLE `assessment_task`
  DROP INDEX `idx_task_org_deleted_created`,
  DROP INDEX `idx_task_org_deleted_open`,
  DROP INDEX `idx_task_org_deleted_completed_status`,
  DROP INDEX `idx_task_org_deleted_expire_status`;

ALTER TABLE `assessment`
  DROP INDEX `idx_assessment_org_deleted_submitted`,
  DROP INDEX `idx_assessment_org_deleted_failed`;

DROP TABLE IF EXISTS `analytics_plan_task_window_snapshot`;

DROP TABLE IF EXISTS `analytics_organization_snapshot`;
