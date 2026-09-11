-- 仅建立业务归属结构；不推断历史归属、不切换数据访问规则。
ALTER TABLE testee
 ADD COLUMN store_id BIGINT UNSIGNED NULL,
 ADD COLUMN store_version INT UNSIGNED NOT NULL DEFAULT 1,
 ADD KEY idx_testee_org_store (org_id, store_id);

CREATE TABLE testee_store_history (
 id BIGINT UNSIGNED NOT NULL PRIMARY KEY,
 org_id BIGINT NOT NULL,
 testee_id BIGINT UNSIGNED NOT NULL,
 from_store_id BIGINT UNSIGNED NULL,
 to_store_id BIGINT UNSIGNED NOT NULL,
 kind VARCHAR(24) NOT NULL,
 actor_id BIGINT NOT NULL,
 entry_id BIGINT UNSIGNED NULL,
 clinician_id BIGINT UNSIGNED NULL,
 created_at DATETIME(6) NOT NULL,
 reason VARCHAR(500) NOT NULL,
 request_id VARCHAR(64) NOT NULL,
 version INT UNSIGNED NOT NULL,
 UNIQUE KEY uk_testee_store_request (org_id, testee_id, request_id),
 KEY idx_testee_store_history (org_id, testee_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
