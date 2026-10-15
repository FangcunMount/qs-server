CREATE TABLE `testee_store_migration_reversions` (
  `migration_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `testee_id` bigint unsigned NOT NULL,
  `org_id` bigint NOT NULL,
  `from_store_id` bigint unsigned NOT NULL,
  `original_history_id` bigint unsigned NOT NULL,
  `restored_version` int unsigned NOT NULL,
  `actor_id` bigint NOT NULL,
  `created_at` datetime(6) NOT NULL,
  PRIMARY KEY (`migration_id`,`testee_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci