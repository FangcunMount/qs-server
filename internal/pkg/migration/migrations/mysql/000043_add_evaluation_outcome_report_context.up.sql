ALTER TABLE `evaluation_outcome`
  ADD COLUMN `org_id` bigint DEFAULT NULL COMMENT 'organization id snapshot' AFTER `id`,
  ADD COLUMN `testee_id` bigint unsigned DEFAULT NULL COMMENT 'testee id snapshot' AFTER `assessment_id`,
  ADD COLUMN `report_input_json` longtext DEFAULT NULL COMMENT 'self-contained model snapshot required for report generation' AFTER `input_snapshot_ref`;

UPDATE `evaluation_outcome` AS `o`
JOIN `assessment` AS `a` ON `a`.`id` = `o`.`assessment_id`
SET `o`.`org_id` = `a`.`org_id`,
    `o`.`testee_id` = `a`.`testee_id`
WHERE `o`.`org_id` IS NULL OR `o`.`testee_id` IS NULL;

ALTER TABLE `evaluation_outcome`
  MODIFY COLUMN `org_id` bigint NOT NULL COMMENT 'organization id snapshot',
  MODIFY COLUMN `testee_id` bigint unsigned NOT NULL COMMENT 'testee id snapshot',
  ADD KEY `idx_evaluation_outcome_org` (`org_id`),
  ADD KEY `idx_evaluation_outcome_testee` (`testee_id`);
