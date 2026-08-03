ALTER TABLE `assessment_task`
  ADD COLUMN `schedule_revision` INT UNSIGNED NOT NULL DEFAULT 1 AFTER `due_at`,
  ADD COLUMN `schedule_defined_at` DATETIME(3) NULL DEFAULT NULL AFTER `schedule_revision`,
  ADD KEY `idx_task_collect_schedule_defined` (`org_id`, `deleted_at`, `schedule_defined_at`, `id`);

ALTER TABLE `statistics_plan_fact`
  ADD COLUMN `schedule_revision` INT UNSIGNED NULL DEFAULT NULL AFTER `task_seq`,
  ADD COLUMN `schedule_planned_at` DATETIME(3) NULL DEFAULT NULL AFTER `planned_at`,
  ADD COLUMN `schedule_due_at` DATETIME(3) NULL DEFAULT NULL AFTER `due_at`,
  ADD KEY `idx_statistics_plan_fact_schedule` (`org_id`, `task_id`, `fact_type`, `schedule_revision`);
