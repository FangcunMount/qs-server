-- Additive MQ perimeter tables, owned by qs-server. No historical row is moved here.
CREATE TABLE ai_messaging_outbox (
  producer VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  destination VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  message_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
  body_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  body MEDIUMBLOB NOT NULL,
  wire MEDIUMBLOB NOT NULL,
  wire_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  kind INT NOT NULL,
  organization_id BIGINT UNSIGNED NOT NULL,
  topic VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  aggregate_key VARCHAR(192) COLLATE utf8mb4_bin NOT NULL,
  aggregate_sequence BIGINT UNSIGNED NOT NULL,
  ordered BOOLEAN NOT NULL,
  requires_receipt BOOLEAN NOT NULL,
  stage VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  attempts BIGINT UNSIGNED NOT NULL DEFAULT 0,
  available_at DATETIME(6) NOT NULL,
  created_at DATETIME(6) NOT NULL,
  published_at DATETIME(6) NULL,
  confirmed_at DATETIME(6) NULL,
  error_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  PRIMARY KEY (producer, destination, message_id),
  INDEX pending_scan (stage, available_at, message_id),
  INDEX aggregate_order (producer, destination, aggregate_key, ordered, aggregate_sequence, stage)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE ai_messaging_aggregates (
 aggregate_key VARCHAR(192) COLLATE utf8mb4_bin PRIMARY KEY,
 next_sequence BIGINT UNSIGNED NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE ai_messaging_operations (
 command_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 kind INT NOT NULL,
 body_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 organization_id BIGINT UNSIGNED NOT NULL,
 subject_id VARCHAR(128) COLLATE utf8mb4_bin NOT NULL,
 resource_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 aggregate_key VARCHAR(192) COLLATE utf8mb4_bin NOT NULL,
 aggregate_sequence BIGINT UNSIGNED NOT NULL,
 decision VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
 code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
 receipt_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL,
 receipt MEDIUMBLOB NULL,
 created_at DATETIME(6) NOT NULL,
 decided_at DATETIME(6) NULL,
 INDEX operation_scope(organization_id,subject_id,command_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE ai_messaging_inbox (
 producer VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 message_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 body_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 body MEDIUMBLOB NOT NULL,
 wire_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 kind INT NOT NULL,
 aggregate_key VARCHAR(192) COLLATE utf8mb4_bin NOT NULL,
 ack_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 received_at DATETIME(6) NOT NULL,
 PRIMARY KEY(producer,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE ai_messaging_evaluation_states (
 run_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 organization_id BIGINT UNSIGNED NOT NULL,
 event_sequence BIGINT UNSIGNED NOT NULL,
 version BIGINT NOT NULL,
 state MEDIUMBLOB NOT NULL,
 updated_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
CREATE TABLE ai_messaging_quarantine (
 wire_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 wire MEDIUMBLOB NOT NULL,
 code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 attempts BIGINT UNSIGNED NOT NULL,
 first_seen_at DATETIME(6) NOT NULL,
 last_seen_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
