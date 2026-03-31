ALTER TABLE `clinician_relation`
  ADD KEY `idx_org_clinician_active_deleted` (`org_id`, `clinician_id`, `is_active`, `deleted_at`),
  ADD KEY `idx_org_testee_active_deleted` (`org_id`, `testee_id`, `is_active`, `deleted_at`);
