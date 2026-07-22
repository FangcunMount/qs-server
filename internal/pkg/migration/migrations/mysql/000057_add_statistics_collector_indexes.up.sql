ALTER TABLE `assessment_entry_resolve_log`
  ADD KEY `idx_entry_resolve_collect` (`org_id`, `deleted_at`, `resolved_at`, `id`);

ALTER TABLE `assessment_entry_intake_log`
  ADD KEY `idx_entry_intake_collect` (`org_id`, `deleted_at`, `intake_at`, `id`);

ALTER TABLE `evaluation_outcome`
  ADD KEY `idx_evaluation_outcome_collect` (`org_id`, `evaluated_at`, `id`);
