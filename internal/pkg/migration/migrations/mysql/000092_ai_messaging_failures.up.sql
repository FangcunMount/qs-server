-- Receiver budget survives physical re-PUB and process restart. Host-owned only.
CREATE TABLE ai_messaging_failures (
 producer VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 message_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 body_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 kind INT NOT NULL,
 aggregate_key VARCHAR(192) COLLATE utf8mb4_bin NOT NULL,
 wire MEDIUMBLOB NOT NULL,
 attempts BIGINT UNSIGNED NOT NULL,
 first_seen_at DATETIME(6) NOT NULL,
 last_seen_at DATETIME(6) NOT NULL,
 PRIMARY KEY(producer,message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
ALTER TABLE ai_messaging_inbox ADD COLUMN outcome VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'stored';
