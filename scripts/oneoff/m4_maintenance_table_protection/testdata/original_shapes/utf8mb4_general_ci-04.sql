CREATE TABLE `operator_retirement_manifests` (
  `migration_id` varchar(64) COLLATE utf8mb4_general_ci NOT NULL,
  `stage` varchar(24) COLLATE utf8mb4_general_ci NOT NULL,
  `payload` longtext COLLATE utf8mb4_general_ci NOT NULL,
  `payload_sha256` char(64) COLLATE utf8mb4_general_ci NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `updated_at` datetime(6) NOT NULL,
  PRIMARY KEY (`migration_id`),
  CONSTRAINT `operator_retirement_manifests_chk_1` CHECK (json_valid(`payload`))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='Operator retirement immutable baseline and resumable execution record'