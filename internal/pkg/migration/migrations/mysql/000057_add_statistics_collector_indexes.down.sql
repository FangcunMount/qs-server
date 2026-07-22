ALTER TABLE `evaluation_outcome`
  DROP KEY `idx_evaluation_outcome_collect`;

ALTER TABLE `assessment_entry_intake_log`
  DROP KEY `idx_entry_intake_collect`;

ALTER TABLE `assessment_entry_resolve_log`
  DROP KEY `idx_entry_resolve_collect`;
