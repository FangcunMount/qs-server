CREATE TABLE `clinician_operator_binding_archive` (
  `clinician_id` bigint unsigned NOT NULL,
  `org_id` bigint NOT NULL,
  `operator_id` bigint unsigned NOT NULL,
  `migration_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `before_record` longtext COLLATE utf8mb4_unicode_ci NOT NULL,
  `record_sha256` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `archived_at` datetime(6) NOT NULL,
  PRIMARY KEY (`clinician_id`),
  KEY `idx_clinician_binding_archive_migration` (`migration_id`),
  CONSTRAINT `clinician_operator_binding_archive_chk_1` CHECK (json_valid(`before_record`))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='Restricted historical clinician backend binding archive'