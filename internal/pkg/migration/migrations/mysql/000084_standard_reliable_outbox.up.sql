-- Host-owned standard Outbox. Apply before enabling the M4 writer or Relay.
-- No historical event_outbox rows are copied or rewritten by this migration.
CREATE TABLE rm_outbox (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  producer VARBINARY(128) NOT NULL,
  message_id VARBINARY(128) NOT NULL,
  destination VARBINARY(255) NOT NULL,
  event_type VARBINARY(255) NOT NULL,
  schema_version VARBINARY(128) NOT NULL,
  scope VARBINARY(255) NOT NULL,
  content_type VARBINARY(255) NOT NULL,
  occurred_at VARBINARY(64) NOT NULL,
  payload LONGBLOB NOT NULL,
  fingerprint BINARY(32) NOT NULL,
  state VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending',
  next_attempt_at DATETIME(6) NOT NULL,
  claim_token VARBINARY(64) NULL,
  lease_until DATETIME(6) NULL,
  version BIGINT UNSIGNED NOT NULL DEFAULT 0,
  attempt_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  failure_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  last_error_code VARCHAR(128) NOT NULL DEFAULT '',
  transport_confirmed_at DATETIME(6) NULL,
  manual_replay_request_id VARBINARY(64) NULL,
  created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
  UNIQUE KEY identity_key (producer,message_id,destination),
  KEY due_idx (state,next_attempt_at,id),
  KEY lease_idx (state,lease_until,id),
  KEY ix_rm_outbox_message_id (message_id,id)
) ENGINE=InnoDB;

-- The request header and all ordered results commit with the Outbox change.
CREATE TABLE qs_rm_replay_requests (
  org_id BIGINT NOT NULL,
  request_id VARBINARY(64) NOT NULL,
  store_name VARBINARY(64) NOT NULL,
  reason VARCHAR(1024) NOT NULL,
  input_hash BINARY(32) NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
  PRIMARY KEY (org_id,request_id),
  KEY ix_qs_rm_replay_requests_org_time (org_id,created_at,request_id)
) ENGINE=InnoDB;

CREATE TABLE qs_rm_replay_items (
  org_id BIGINT NOT NULL,
  request_id VARBINARY(64) NOT NULL,
  ordinal SMALLINT UNSIGNED NOT NULL,
  event_id VARBINARY(128) NOT NULL,
  expected_failure_count BIGINT UNSIGNED NOT NULL,
  authorized TINYINT(1) NOT NULL,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  PRIMARY KEY (org_id,request_id,ordinal),
  KEY ix_qs_rm_replay_items_event (event_id),
  CONSTRAINT fk_qs_rm_replay_request FOREIGN KEY (org_id,request_id)
    REFERENCES qs_rm_replay_requests(org_id,request_id)
) ENGINE=InnoDB;
