ALTER TABLE `assessment_task`
  ADD COLUMN `due_at` DATETIME(3) NULL DEFAULT NULL AFTER `planned_at`,
  ADD COLUMN `expiration_reason` VARCHAR(32) NULL DEFAULT NULL AFTER `expired_at`,
  ADD KEY `idx_task_org_entry_expire_scan` (`org_id`, `status`, `deleted_at`, `expire_at`, `id`);

ALTER TABLE `assessment`
  ADD KEY `idx_assessment_org_testee_evaluated_summary` (`org_id`, `testee_id`, `status`, `deleted_at`, `evaluated_at`, `id`);
