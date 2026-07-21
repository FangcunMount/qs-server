ALTER TABLE `statistics_sync_run`
  DROP KEY `idx_statistics_sync_run_org_status_started`,
  DROP COLUMN `last_cache_resume_status`,
  DROP COLUMN `last_cache_resume_at`,
  DROP COLUMN `last_cache_resume_reason`,
  DROP COLUMN `last_cache_resume_operator_id`,
  DROP COLUMN `cache_resume_count`,
  DROP COLUMN `run_mode`;
