CREATE TABLE IF NOT EXISTS `evaluation_run` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `run_id` varchar(100) NOT NULL COMMENT '执行运行ID',
  `assessment_id` bigint unsigned NOT NULL COMMENT '测评ID',
  `attempt_no` int unsigned NOT NULL COMMENT '尝试序号',
  `status` varchar(50) NOT NULL COMMENT '运行状态',
  `started_at` datetime NOT NULL COMMENT '开始时间',
  `finished_at` datetime DEFAULT NULL COMMENT '结束时间',
  `error_code` varchar(50) DEFAULT NULL COMMENT '错误分类',
  `error_message` varchar(500) DEFAULT NULL COMMENT '错误信息',
  `retryable` tinyint(1) NOT NULL DEFAULT '0' COMMENT '是否可重试',
  `trace_id` varchar(100) DEFAULT NULL COMMENT '链路追踪ID',
  `input_snapshot_ref` varchar(200) DEFAULT NULL COMMENT 'input snapshot ref',
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_evaluation_run_id` (`run_id`),
  KEY `idx_evaluation_run_assessment_id` (`assessment_id`),
  KEY `idx_evaluation_run_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='测评执行运行记录';

INSERT INTO `evaluation_run` (
  `run_id`, `assessment_id`, `attempt_no`, `status`, `started_at`, `finished_at`,
  `error_code`, `error_message`, `retryable`, `trace_id`, `input_snapshot_ref`, `created_at`, `updated_at`
)
SELECT
  `resource_id`,
  `assessment_id`,
  `attempt_no`,
  `status`,
  `started_at`,
  `finished_at`,
  `error_code`,
  `error_message`,
  `retryable`,
  `trace_id`,
  `input_snapshot_ref`,
  `created_at`,
  `updated_at`
FROM `runtime_checkpoint`
WHERE `scope` = 'evaluation_run' AND `deleted_at` IS NULL;

CREATE TABLE IF NOT EXISTS `analytics_projector_checkpoint` (
  `event_id` VARCHAR(128) NOT NULL,
  `event_type` VARCHAR(128) NOT NULL,
  `status` VARCHAR(32) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`event_id`),
  KEY `idx_analytics_projector_checkpoint_status` (`status`),
  KEY `idx_analytics_projector_checkpoint_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `analytics_projector_checkpoint` (
  `event_id`, `event_type`, `status`, `created_at`, `updated_at`, `deleted_at`
)
SELECT
  `resource_id`,
  `event_type`,
  CASE `status`
    WHEN 'running' THEN 'processing'
    WHEN 'succeeded' THEN 'completed'
    ELSE `status`
  END,
  `created_at`,
  `updated_at`,
  `deleted_at`
FROM `runtime_checkpoint`
WHERE `scope` = 'analytics_projector';

DROP TABLE IF EXISTS `runtime_checkpoint`;
