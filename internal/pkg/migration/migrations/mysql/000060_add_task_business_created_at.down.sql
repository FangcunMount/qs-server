ALTER TABLE `assessment_task`
  DROP INDEX `idx_task_collect_business_created`,
  DROP COLUMN `business_created_at`;
