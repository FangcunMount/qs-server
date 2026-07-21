ALTER TABLE `statistics_sync_run`
  ADD COLUMN `cache_generation` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `as_of_date`,
  ADD COLUMN `cache_published_at` DATETIME(3) NULL AFTER `cache_generation`,
  ADD COLUMN `cache_resume_audit_json` JSON NULL AFTER `last_cache_resume_status`,
  ADD KEY `idx_statistics_sync_run_org_publication` (`org_id`,`run_mode`,`status`,`id`);
