CREATE TABLE `testee_store_migration_items` (
  `migration_id` varchar(64) NOT NULL,
  `testee_id` bigint unsigned NOT NULL,
  `org_id` bigint NOT NULL,
  `before_store_id` bigint unsigned DEFAULT NULL,
  `target_store_id` bigint unsigned DEFAULT NULL,
  `before_version` int unsigned NOT NULL,
  `applied_version` int unsigned NOT NULL,
  `history_id` bigint unsigned DEFAULT NULL,
  `disposition` varchar(24) NOT NULL,
  `reason` varchar(100) NOT NULL,
  PRIMARY KEY (`migration_id`,`testee_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci