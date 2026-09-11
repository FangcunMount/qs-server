CREATE TABLE testee_store_migration_manifests (
 migration_id VARCHAR(64) NOT NULL PRIMARY KEY,
 state VARCHAR(24) NOT NULL,
 before_hash CHAR(64) NOT NULL,
 after_hash CHAR(64) NOT NULL DEFAULT '',
 rollback_hash CHAR(64) NOT NULL DEFAULT '',
 items_hash CHAR(64) NOT NULL,
 item_count BIGINT UNSIGNED NOT NULL,
 actor_id BIGINT NOT NULL,
 created_at DATETIME(6) NOT NULL,
 completed_at DATETIME(6) NULL,
 rolled_back_at DATETIME(6) NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE testee_store_migration_items (
 migration_id VARCHAR(64) NOT NULL,
 testee_id BIGINT UNSIGNED NOT NULL,
 org_id BIGINT NOT NULL,
 before_store_id BIGINT UNSIGNED NULL,
 target_store_id BIGINT UNSIGNED NULL,
 before_version INT UNSIGNED NOT NULL,
 applied_version INT UNSIGNED NOT NULL,
 history_id BIGINT UNSIGNED NULL,
 disposition VARCHAR(24) NOT NULL,
 reason VARCHAR(100) NOT NULL,
 PRIMARY KEY (migration_id,testee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE testee_store_migration_reversions (
 migration_id VARCHAR(64) NOT NULL,
 testee_id BIGINT UNSIGNED NOT NULL,
 org_id BIGINT NOT NULL,
 from_store_id BIGINT UNSIGNED NOT NULL,
 original_history_id BIGINT UNSIGNED NOT NULL,
 restored_version INT UNSIGNED NOT NULL,
 actor_id BIGINT NOT NULL,
 created_at DATETIME(6) NOT NULL,
 PRIMARY KEY (migration_id,testee_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
