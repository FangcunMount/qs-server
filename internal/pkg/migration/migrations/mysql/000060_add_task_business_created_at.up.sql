ALTER TABLE `assessment_task`
  ADD COLUMN `business_created_at` DATETIME(3) NULL DEFAULT NULL AFTER `scale_code`,
  ADD KEY `idx_task_collect_business_created` (`org_id`, `deleted_at`, `business_created_at`, `id`);
