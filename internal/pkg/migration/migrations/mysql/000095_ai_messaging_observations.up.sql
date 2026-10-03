-- Fixed technical facts only; never identities, payloads, budgets or retry authority.
CREATE TABLE ai_messaging_observations (
 kind VARCHAR(64) COLLATE ascii_bin NOT NULL PRIMARY KEY,
 recorded_count BIGINT UNSIGNED NOT NULL,
 recording_since DATETIME(6) NOT NULL,
 last_observed_at DATETIME(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
INSERT INTO ai_messaging_observations(kind,recorded_count,recording_since) VALUES
 ('duplicate_event',0,UTC_TIMESTAMP(6)),
 ('payload_fetch_unavailable',0,UTC_TIMESTAMP(6)),
 ('payload_fetch_reference_mismatch',0,UTC_TIMESTAMP(6)),
 ('payload_fetch_workload_denied',0,UTC_TIMESTAMP(6)),
 ('payload_serve_reference_mismatch',0,UTC_TIMESTAMP(6)),
 ('payload_serve_workload_denied',0,UTC_TIMESTAMP(6)),
 ('payload_serve_storage_unavailable',0,UTC_TIMESTAMP(6));
