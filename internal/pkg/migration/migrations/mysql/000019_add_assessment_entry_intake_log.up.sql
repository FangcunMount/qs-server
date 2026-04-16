CREATE TABLE IF NOT EXISTS `assessment_entry_intake_log` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `clinician_id` BIGINT UNSIGNED NOT NULL,
  `entry_id` BIGINT UNSIGNED NOT NULL,
  `testee_id` BIGINT UNSIGNED NOT NULL,
  `testee_created` TINYINT(1) NOT NULL DEFAULT 0,
  `assignment_created` TINYINT(1) NOT NULL DEFAULT 0,
  `intake_at` DATETIME(3) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_entry_intake_org_entry_time` (`org_id`, `entry_id`, `intake_at`),
  KEY `idx_entry_intake_clinician_time` (`clinician_id`, `intake_at`),
  KEY `idx_entry_intake_org_testee_time` (`org_id`, `testee_id`, `intake_at`),
  KEY `idx_entry_intake_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
