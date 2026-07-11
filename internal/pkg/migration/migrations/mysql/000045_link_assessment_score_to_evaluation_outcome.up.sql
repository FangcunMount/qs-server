-- assessment_score is a query projection. Link every newly projected row to
-- the immutable EvaluationOutcome that is its canonical scoring fact.
ALTER TABLE `assessment_score`
  ADD COLUMN `evaluation_outcome_id` bigint unsigned DEFAULT NULL COMMENT 'canonical evaluation outcome id' AFTER `assessment_id`,
  ADD KEY `idx_assessment_score_outcome` (`evaluation_outcome_id`);

-- Backfill rows whose immutable source was already persisted before this
-- migration. Rows without an outcome remain legacy projections and are not a
-- source of truth for single-assessment score reads.
UPDATE `assessment_score` AS score
INNER JOIN `evaluation_outcome` AS outcome ON outcome.assessment_id = score.assessment_id
SET score.evaluation_outcome_id = outcome.id
WHERE score.evaluation_outcome_id IS NULL;
