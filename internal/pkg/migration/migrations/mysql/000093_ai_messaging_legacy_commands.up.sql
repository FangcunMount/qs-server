-- Immutable handoff evidence; delivered is not repurposed as transport ownership.
CREATE TABLE ai_messaging_legacy_commands (
 command_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 request_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 source_kind VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 source_payload MEDIUMBLOB NOT NULL,
 source_payload_hash CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 source_attempts INT NOT NULL,
 source_available_at DATETIME(6) NOT NULL,
 source_original_time VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 messaging_body_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 transferred_at DATETIME(6) NOT NULL,
 INDEX legacy_aggregate(request_id,command_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
