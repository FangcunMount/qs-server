CREATE TABLE operator_retirement_manifests (
 migration_id VARCHAR(64) NOT NULL PRIMARY KEY,
 stage VARCHAR(24) NOT NULL,
 payload LONGTEXT NOT NULL CHECK (JSON_VALID(payload)),
 payload_sha256 CHAR(64) NOT NULL,
 created_at DATETIME(6) NOT NULL,
 updated_at DATETIME(6) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Operator retirement immutable baseline and resumable execution record';
CREATE TABLE clinician_operator_binding_archive (
 clinician_id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
 org_id BIGINT NOT NULL,
 operator_id BIGINT UNSIGNED NOT NULL,
 migration_id VARCHAR(64) NOT NULL,
 before_record LONGTEXT NOT NULL CHECK (JSON_VALID(before_record)),
 record_sha256 CHAR(64) NOT NULL,
 archived_at DATETIME(6) NOT NULL,
 KEY idx_clinician_binding_archive_migration(migration_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Restricted historical clinician backend binding archive';
