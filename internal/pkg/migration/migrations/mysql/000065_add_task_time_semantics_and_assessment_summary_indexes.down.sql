ALTER TABLE `assessment`
  DROP INDEX `idx_assessment_org_testee_evaluated_summary`;

ALTER TABLE `assessment_task`
  DROP INDEX `idx_task_org_entry_expire_scan`,
  DROP COLUMN `expiration_reason`,
  DROP COLUMN `due_at`;
