CREATE TABLE IF NOT EXISTS `analytics_organization_snapshot` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `testee_count` BIGINT NOT NULL DEFAULT 0,
  `clinician_count` BIGINT NOT NULL DEFAULT 0,
  `active_entry_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_count` BIGINT NOT NULL DEFAULT 0,
  `report_count` BIGINT NOT NULL DEFAULT 0,
  `dimension_clinician_count` BIGINT NOT NULL DEFAULT 0,
  `dimension_entry_count` BIGINT NOT NULL DEFAULT 0,
  `dimension_content_count` BIGINT NOT NULL DEFAULT 0,
  `snapshot_at` DATETIME(3) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_organization_snapshot_org` (`org_id`),
  KEY `idx_analytics_organization_snapshot_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='机构统计快照';

CREATE TABLE IF NOT EXISTS `analytics_plan_task_window_snapshot` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `preset` VARCHAR(20) NOT NULL,
  `window_start` DATE NOT NULL,
  `window_end` DATE NOT NULL,
  `task_created_count` BIGINT NOT NULL DEFAULT 0,
  `task_opened_count` BIGINT NOT NULL DEFAULT 0,
  `task_completed_count` BIGINT NOT NULL DEFAULT 0,
  `task_expired_count` BIGINT NOT NULL DEFAULT 0,
  `enrolled_testees` BIGINT NOT NULL DEFAULT 0,
  `active_testees` BIGINT NOT NULL DEFAULT 0,
  `snapshot_at` DATETIME(3) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_plan_task_window_snapshot` (`org_id`, `preset`, `window_start`, `window_end`),
  KEY `idx_analytics_plan_task_window_org_range` (`org_id`, `window_start`, `window_end`),
  KEY `idx_analytics_plan_task_window_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='计划任务常用窗口快照';

ALTER TABLE `assessment`
  ADD INDEX `idx_assessment_org_deleted_submitted` (`org_id`, `deleted_at`, `submitted_at`),
  ADD INDEX `idx_assessment_org_deleted_failed` (`org_id`, `deleted_at`, `failed_at`);

ALTER TABLE `assessment_task`
  ADD INDEX `idx_task_org_deleted_created` (`org_id`, `deleted_at`, `created_at`, `plan_id`, `testee_id`),
  ADD INDEX `idx_task_org_deleted_open` (`org_id`, `deleted_at`, `open_at`, `plan_id`, `testee_id`),
  ADD INDEX `idx_task_org_deleted_completed_status` (`org_id`, `deleted_at`, `completed_at`, `status`, `plan_id`, `testee_id`),
  ADD INDEX `idx_task_org_deleted_expire_status` (`org_id`, `deleted_at`, `expire_at`, `status`, `plan_id`, `testee_id`);
