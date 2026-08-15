CREATE TABLE `mongo_consistency_audit_checkpoint` (
  `checkpoint_key` varchar(64) NOT NULL,
  `schema_version` int NOT NULL,
  `revision` bigint NOT NULL,
  `cycle_id` varchar(64) NOT NULL,
  `phase` varchar(64) NOT NULL,
  `cursor` bigint unsigned NOT NULL DEFAULT 0,
  `cycle_upper_bound` bigint unsigned NOT NULL DEFAULT 0,
  `statistics_json` json NOT NULL,
  `last_completed_json` json DEFAULT NULL,
  `next_cycle_at` datetime(3) DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`checkpoint_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
