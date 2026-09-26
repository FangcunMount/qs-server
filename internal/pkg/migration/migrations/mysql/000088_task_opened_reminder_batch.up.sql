-- Freeze the whole recipient identity set for one task opening. Neither
-- OpenID nor the bearer entry URL is stored here.
CREATE TABLE task_opened_reminder_batch (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  org_id BIGINT NOT NULL,
  task_id VARBINARY(64) NOT NULL,
  opening_event_id VARBINARY(64) NOT NULL,
  schedule_revision INT UNSIGNED NOT NULL,
  reminder_version INT UNSIGNED NOT NULL,
  app_id VARBINARY(128) NOT NULL,
  template_id VARBINARY(128) NOT NULL,
  recipient_count INT UNSIGNED NOT NULL,
  state VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  resolution_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  UNIQUE KEY uk_task_opened_reminder_batch (
    org_id, task_id, opening_event_id, schedule_revision, reminder_version
  ),
  KEY ix_task_opened_reminder_batch_review (org_id, state, updated_at, id)
) ENGINE=InnoDB;
