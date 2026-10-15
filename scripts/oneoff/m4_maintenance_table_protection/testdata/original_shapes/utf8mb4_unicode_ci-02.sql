CREATE TABLE `testee_store_migration_manifests` (
  `migration_id` varchar(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `state` varchar(24) COLLATE utf8mb4_unicode_ci NOT NULL,
  `before_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `after_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `rollback_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `items_hash` char(64) COLLATE utf8mb4_unicode_ci NOT NULL,
  `item_count` bigint unsigned NOT NULL,
  `actor_id` bigint NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `completed_at` datetime(6) DEFAULT NULL,
  `rolled_back_at` datetime(6) DEFAULT NULL,
  PRIMARY KEY (`migration_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci