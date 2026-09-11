CREATE TABLE ai_bridge_requests (
 request_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 request_hash CHAR(64) NOT NULL, payload JSON NOT NULL,
 session_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NULL UNIQUE,
 version BIGINT NOT NULL DEFAULT 0, status VARCHAR(32) NOT NULL DEFAULT 'pending',
 projection JSON NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE ai_bridge_commands (
 command_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 request_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 kind VARCHAR(16) NOT NULL, payload JSON NOT NULL, payload_hash CHAR(64) NOT NULL,
 delivered BOOLEAN NOT NULL DEFAULT FALSE, attempts INT NOT NULL DEFAULT 0,
 available_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 FOREIGN KEY (request_id) REFERENCES ai_bridge_requests(request_id),
 INDEX ix_ai_bridge_due(delivered,available_at,command_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE ai_bridge_events (
 event_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin PRIMARY KEY,
 request_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 version BIGINT NOT NULL, payload_hash CHAR(64) NOT NULL,
 UNIQUE KEY uq_ai_bridge_event_version(request_id,version),
 FOREIGN KEY (request_id) REFERENCES ai_bridge_requests(request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
