CREATE TABLE IF NOT EXISTS `assessment_entry_resolve_log` (
  `id` BIGINT UNSIGNED NOT NULL,
  `org_id` BIGINT NOT NULL,
  `clinician_id` BIGINT UNSIGNED NOT NULL,
  `entry_id` BIGINT UNSIGNED NOT NULL,
  `resolved_at` DATETIME(3) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_entry_resolve_org_entry_time` (`org_id`, `entry_id`, `resolved_at`),
  KEY `idx_entry_resolve_clinician_time` (`clinician_id`, `resolved_at`),
  KEY `idx_entry_resolve_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
