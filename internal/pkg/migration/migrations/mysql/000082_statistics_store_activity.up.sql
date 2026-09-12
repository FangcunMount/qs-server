CREATE TABLE statistics_store_activity_fact (
 id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
 org_id BIGINT NOT NULL,
 fact_key VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 fact_type VARCHAR(40) NOT NULL,
 business_object_id BIGINT UNSIGNED NOT NULL,
 occurred_at DATETIME(6) NOT NULL,
 stat_date DATE NOT NULL,
 conducting_store_id BIGINT UNSIGNED NULL,
 unknown_reason VARCHAR(40) NOT NULL DEFAULT '',
 source_type VARCHAR(40) NOT NULL,
 source_ref VARCHAR(128) NOT NULL,
 schema_version INT UNSIGNED NOT NULL,
 core_hash CHAR(64) CHARACTER SET ascii NOT NULL,
 created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 UNIQUE KEY uk_store_activity_fact_key(fact_key),
 UNIQUE KEY uk_store_activity_business(org_id,fact_type,business_object_id),
 KEY idx_store_activity_window(org_id,stat_date,conducting_store_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE statistics_store_activity_daily (
 org_id BIGINT NOT NULL,
 stat_date DATE NOT NULL,
 conducting_store_id BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '0 is an explicit unknown bucket, never company scope',
 unknown_reason VARCHAR(40) NOT NULL DEFAULT '',
 answersheet_submitted_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
 assessment_completed_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
 PRIMARY KEY(org_id,stat_date,conducting_store_id,unknown_reason)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
