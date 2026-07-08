-- Merge evaluation_run and analytics_projector_checkpoint into runtime_checkpoint.
-- Check before apply:
--   SELECT COUNT(*) FROM evaluation_run;
--   SELECT COUNT(*) FROM analytics_projector_checkpoint WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS `runtime_checkpoint` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `scope` varchar(64) NOT NULL COMMENT 'checkpoint scope',
  `resource_id` varchar(128) NOT NULL COMMENT 'scope resource id',
  `attempt_no` int unsigned NOT NULL DEFAULT '1' COMMENT 'attempt number',
  `assessment_id` bigint unsigned DEFAULT NULL COMMENT 'evaluation assessment id',
  `event_type` varchar(128) DEFAULT NULL COMMENT 'analytics event type',
  `status` varchar(50) NOT NULL COMMENT 'checkpoint status',
  `started_at` datetime(3) NOT NULL COMMENT 'start time',
  `finished_at` datetime(3) DEFAULT NULL COMMENT 'finish time',
  `error_code` varchar(50) DEFAULT NULL COMMENT 'error code',
  `error_message` varchar(500) DEFAULT NULL COMMENT 'error message',
  `retryable` tinyint(1) NOT NULL DEFAULT '0' COMMENT 'retryable flag',
  `trace_id` varchar(100) DEFAULT NULL COMMENT 'trace id',
  `input_snapshot_ref` varchar(200) DEFAULT NULL COMMENT 'input snapshot ref',
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT 'created at',
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT 'updated at',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT 'soft delete',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_runtime_checkpoint_scope_resource_attempt` (`scope`, `resource_id`, `attempt_no`),
  KEY `idx_runtime_checkpoint_scope_status` (`scope`, `status`),
  KEY `idx_runtime_checkpoint_assessment_id` (`assessment_id`),
  KEY `idx_runtime_checkpoint_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='unified runtime checkpoint';

INSERT INTO `runtime_checkpoint` (
  `scope`, `resource_id`, `attempt_no`, `assessment_id`, `event_type`, `status`,
  `started_at`, `finished_at`, `error_code`, `error_message`, `retryable`,
  `trace_id`, `input_snapshot_ref`, `created_at`, `updated_at`, `deleted_at`
)
SELECT
  'evaluation_run',
  `run_id`,
  `attempt_no`,
  `assessment_id`,
  NULL,
  `status`,
  `started_at`,
  `finished_at`,
  `error_code`,
  `error_message`,
  `retryable`,
  `trace_id`,
  `input_snapshot_ref`,
  `created_at`,
  `updated_at`,
  NULL
FROM `evaluation_run`;

INSERT INTO `runtime_checkpoint` (
  `scope`, `resource_id`, `attempt_no`, `assessment_id`, `event_type`, `status`,
  `started_at`, `finished_at`, `error_code`, `error_message`, `retryable`,
  `trace_id`, `input_snapshot_ref`, `created_at`, `updated_at`, `deleted_at`
)
SELECT
  'analytics_projector',
  `event_id`,
  1,
  NULL,
  `event_type`,
  CASE `status`
    WHEN 'processing' THEN 'running'
    WHEN 'completed' THEN 'succeeded'
    ELSE `status`
  END,
  `created_at`,
  `updated_at`,
  NULL,
  NULL,
  0,
  NULL,
  NULL,
  `created_at`,
  `updated_at`,
  `deleted_at`
FROM `analytics_projector_checkpoint`;

DROP TABLE IF EXISTS `analytics_projector_checkpoint`;
DROP TABLE IF EXISTS `evaluation_run`;
