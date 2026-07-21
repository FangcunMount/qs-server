ALTER TABLE `statistics_sync_run`
  ADD COLUMN `run_mode` VARCHAR(32) NOT NULL DEFAULT 'publish' AFTER `trigger_type`,
  ADD COLUMN `cache_resume_count` INT UNSIGNED NOT NULL DEFAULT 0 AFTER `error_message`,
  ADD COLUMN `last_cache_resume_operator_id` BIGINT UNSIGNED NULL AFTER `cache_resume_count`,
  ADD COLUMN `last_cache_resume_reason` VARCHAR(500) NOT NULL DEFAULT '' AFTER `last_cache_resume_operator_id`,
  ADD COLUMN `last_cache_resume_at` DATETIME(3) NULL AFTER `last_cache_resume_reason`,
  ADD COLUMN `last_cache_resume_status` VARCHAR(32) NOT NULL DEFAULT '' AFTER `last_cache_resume_at`,
  ADD KEY `idx_statistics_sync_run_org_status_started` (`org_id`,`status`,`started_at`);
