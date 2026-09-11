CREATE TABLE actor_stores (
 id BIGINT UNSIGNED NOT NULL PRIMARY KEY, org_id BIGINT NOT NULL,
 code VARCHAR(32) NOT NULL, name VARCHAR(100) NOT NULL, address VARCHAR(255) NOT NULL DEFAULT '',
 is_active BOOLEAN NOT NULL DEFAULT TRUE, version INT UNSIGNED NOT NULL DEFAULT 1,
 created_at DATETIME(6) NOT NULL, updated_at DATETIME(6) NOT NULL,
 created_by BIGINT NOT NULL, updated_by BIGINT NOT NULL,
 UNIQUE KEY uk_store_org_code (org_id,code), KEY idx_store_org_active(org_id,is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
ALTER TABLE clinician ADD COLUMN store_id BIGINT UNSIGNED NULL, ADD KEY idx_clinician_org_store(org_id,store_id);
ALTER TABLE assessment_entry ADD COLUMN invalidated_at DATETIME(6) NULL, ADD COLUMN invalidation_reason VARCHAR(64) NOT NULL DEFAULT '';
CREATE TABLE clinician_store_history (
 id BIGINT UNSIGNED NOT NULL PRIMARY KEY, org_id BIGINT NOT NULL, clinician_id BIGINT UNSIGNED NOT NULL,
 from_store_id BIGINT UNSIGNED NULL, to_store_id BIGINT UNSIGNED NOT NULL, kind VARCHAR(16) NOT NULL,
 actor_id BIGINT NOT NULL, created_at DATETIME(6) NOT NULL, reason VARCHAR(500) NOT NULL,
 request_id VARCHAR(64) NOT NULL, invalidated_count BIGINT NOT NULL DEFAULT 0, version INT UNSIGNED NOT NULL,
 UNIQUE KEY uk_clinician_store_request(org_id,clinician_id,request_id), KEY idx_clinician_store_history(org_id,clinician_id,created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
