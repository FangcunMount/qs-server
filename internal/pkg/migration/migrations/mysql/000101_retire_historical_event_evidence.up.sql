ALTER TABLE `assessment`
  DROP CHECK `chk_assessment_historical_lifecycle`,
  DROP COLUMN `historical_lifecycle_evidence`;

ALTER TABLE `evaluation_outcome`
  DROP CHECK `chk_outcome_historical_committed`,
  DROP COLUMN `historical_committed_evidence`;
