CREATE TABLE IF NOT EXISTS `seed_backfill_rollback_operation` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `batch_id` VARCHAR(96) NOT NULL,
  `manifest_hash` CHAR(64) NOT NULL,
  `scope_hash` CHAR(64) NOT NULL,
  `backup_suffix` VARCHAR(32) NOT NULL,
  `phase` VARCHAR(32) NOT NULL,
  `status` VARCHAR(24) NOT NULL,
  `last_error` VARCHAR(2000) NULL,
  `started_at` DATETIME(6) NOT NULL,
  `completed_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_seed_backfill_rollback_batch_scope` (`org_id`, `batch_id`, `scope_hash`),
  KEY `idx_seed_backfill_rollback_batch_status` (`org_id`, `batch_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `seed_backfill_rollback_resource` (
  `operation_id` BIGINT UNSIGNED NOT NULL,
  `storage` VARCHAR(16) NOT NULL,
  `resource_type` VARCHAR(64) NOT NULL,
  `resource_id` VARCHAR(255) NOT NULL,
  `business_date` DATE NULL,
  `metadata_json` JSON NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`operation_id`, `storage`, `resource_type`, `resource_id`),
  KEY `idx_seed_backfill_rollback_resource_lookup` (`storage`, `resource_type`, `resource_id`),
  KEY `idx_seed_backfill_rollback_resource_date` (`operation_id`, `business_date`),
  CONSTRAINT `fk_seed_backfill_rollback_resource_operation`
    FOREIGN KEY (`operation_id`) REFERENCES `seed_backfill_rollback_operation` (`id`)
    ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `seed_backfill_rollback_phase_attempt` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `operation_id` BIGINT UNSIGNED NOT NULL,
  `phase` VARCHAR(32) NOT NULL,
  `attempt_no` INT UNSIGNED NOT NULL,
  `status` VARCHAR(24) NOT NULL,
  `error_text` VARCHAR(2000) NULL,
  `started_at` DATETIME(6) NOT NULL,
  `finished_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_seed_backfill_rollback_phase_attempt` (`operation_id`, `phase`, `attempt_no`),
  KEY `idx_seed_backfill_rollback_phase_status` (`operation_id`, `status`),
  CONSTRAINT `fk_seed_backfill_rollback_phase_operation`
    FOREIGN KEY (`operation_id`) REFERENCES `seed_backfill_rollback_operation` (`id`)
    ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
