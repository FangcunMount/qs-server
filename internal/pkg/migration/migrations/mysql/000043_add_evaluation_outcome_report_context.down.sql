ALTER TABLE `evaluation_outcome`
  DROP INDEX `idx_evaluation_outcome_org`,
  DROP INDEX `idx_evaluation_outcome_testee`,
  DROP COLUMN `report_input_json`,
  DROP COLUMN `testee_id`,
  DROP COLUMN `org_id`;
