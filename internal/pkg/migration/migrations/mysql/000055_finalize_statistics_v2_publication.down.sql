ALTER TABLE `statistics_sync_run`
  DROP KEY `idx_statistics_sync_run_org_publication`,
  DROP COLUMN `cache_resume_audit_json`,
  DROP COLUMN `cache_published_at`,
  DROP COLUMN `cache_generation`;
