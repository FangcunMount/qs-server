ALTER TABLE `statistics_plan_fact`
  DROP INDEX `idx_statistics_plan_fact_schedule`,
  DROP COLUMN `schedule_due_at`,
  DROP COLUMN `schedule_planned_at`,
  DROP COLUMN `schedule_revision`;

ALTER TABLE `assessment_task`
  DROP INDEX `idx_task_collect_schedule_defined`,
  DROP COLUMN `schedule_defined_at`,
  DROP COLUMN `schedule_revision`;
