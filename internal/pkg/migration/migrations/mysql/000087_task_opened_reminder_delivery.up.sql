-- Per-recipient responsibility for the durable task-opened reminder.
-- This table is inert until the new consumer is explicitly wired and enabled.
CREATE TABLE task_opened_reminder_delivery (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  org_id BIGINT NOT NULL,
  task_id VARBINARY(64) NOT NULL,
  opening_event_id VARBINARY(64) NOT NULL,
  schedule_revision INT UNSIGNED NOT NULL,
  user_id VARBINARY(64) NOT NULL,
  login_identity_id VARBINARY(64) NOT NULL,
  app_id VARBINARY(128) NOT NULL,
  template_id VARBINARY(128) NOT NULL,
  reminder_version INT UNSIGNED NOT NULL,
  state VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  claim_token VARBINARY(64) NULL,
  lease_until DATETIME(6) NULL,
  external_call_started_at DATETIME(6) NULL,
  platform_message_id VARBINARY(64) NULL,
  resolution_code VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  created_at DATETIME(6) NOT NULL,
  updated_at DATETIME(6) NOT NULL,
  UNIQUE KEY uk_task_opened_reminder_recipient (
    org_id, task_id, opening_event_id, schedule_revision,
    login_identity_id, app_id, reminder_version
  ),
  KEY ix_task_opened_reminder_review (org_id, state, updated_at, id),
  KEY ix_task_opened_reminder_lease (state, lease_until, id)
) ENGINE=InnoDB;
