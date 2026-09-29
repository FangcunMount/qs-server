-- One reviewed request, one original evaluation event, one durable outcome.
-- This table does not create or replay any message by itself.
CREATE TABLE qs_rm_gap_recovery_request (
  org_id BIGINT NOT NULL,
  request_id VARBINARY(64) NOT NULL,
  actor_id BIGINT UNSIGNED NOT NULL,
  assessment_id BIGINT UNSIGNED NOT NULL,
  event_id VARBINARY(128) NOT NULL,
  expected_version BIGINT UNSIGNED NOT NULL,
  reason VARCHAR(1024) NOT NULL,
  submitted_before VARBINARY(64) NOT NULL,
  input_hash BINARY(32) NOT NULL,
  result_code VARCHAR(64) NOT NULL DEFAULT '',
  authorized TINYINT(1) NOT NULL DEFAULT 0,
  outbox_version_before BIGINT UNSIGNED NULL,
  outbox_version_after BIGINT UNSIGNED NULL,
  created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
  updated_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
  PRIMARY KEY (org_id,request_id),
  KEY ix_qs_rm_gap_recovery_assessment (assessment_id,created_at,request_id)
) ENGINE=InnoDB;
