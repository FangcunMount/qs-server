-- EvaluationRun persistence (phase A): run records + assessment current_run_id pointer.
-- Check before apply:
--   SELECT COUNT(*) FROM assessment;

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
  `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_evaluation_run_id` (`run_id`),
  KEY `idx_evaluation_run_assessment_id` (`assessment_id`),
  KEY `idx_evaluation_run_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='测评执行运行记录';

ALTER TABLE `assessment`
    ADD COLUMN `current_run_id` varchar(100) DEFAULT NULL COMMENT '当前执行运行ID' AFTER `failure_reason`,
    ADD INDEX `idx_assessment_current_run_id` (`current_run_id`);
