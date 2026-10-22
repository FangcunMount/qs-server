-- Explicit host installation for current RocketMQ deferred consumption.
-- Version1 rows retain their old responsibility; the v2 reader never adopts
-- them. Pending legacy work is resolved during the coordinated handoff.
ALTER TABLE retry_event_hold
 ADD COLUMN protocol_version SMALLINT UNSIGNED NOT NULL DEFAULT 1,
 ADD COLUMN transport_identity_hash BINARY(32) NULL,
 ADD COLUMN received_timestamp BIGINT NOT NULL DEFAULT 0,
 ADD COLUMN consumed_at DATETIME(3) NULL,
 ADD COLUMN used_manual_replay_request_id VARCHAR(64) NULL,
 ADD KEY idx_retry_hold_current_consumer (provider,channel_name,protocol_version,status,retry_disposition,next_attempt_at,claim_expires_at);
