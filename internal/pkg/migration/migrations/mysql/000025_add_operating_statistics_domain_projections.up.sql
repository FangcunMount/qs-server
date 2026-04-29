CREATE TABLE IF NOT EXISTS `analytics_access_org_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `stat_date` DATE NOT NULL,
  `entry_opened_count` BIGINT NOT NULL DEFAULT 0,
  `intake_confirmed_count` BIGINT NOT NULL DEFAULT 0,
  `testee_created_count` BIGINT NOT NULL DEFAULT 0,
  `care_relationship_established_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_access_org_daily` (`org_id`, `stat_date`),
  KEY `idx_analytics_access_org_date` (`org_id`, `stat_date`),
  KEY `idx_analytics_access_org_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='接入漏斗机构日投影';

CREATE TABLE IF NOT EXISTS `analytics_access_clinician_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `clinician_id` BIGINT UNSIGNED NOT NULL,
  `stat_date` DATE NOT NULL,
  `entry_opened_count` BIGINT NOT NULL DEFAULT 0,
  `intake_confirmed_count` BIGINT NOT NULL DEFAULT 0,
  `testee_created_count` BIGINT NOT NULL DEFAULT 0,
  `care_relationship_established_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_access_clinician_daily` (`org_id`, `clinician_id`, `stat_date`),
  KEY `idx_analytics_access_clinician_date` (`org_id`, `stat_date`),
  KEY `idx_analytics_access_clinician_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='接入漏斗从业者日投影';

CREATE TABLE IF NOT EXISTS `analytics_access_entry_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `entry_id` BIGINT UNSIGNED NOT NULL,
  `clinician_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `stat_date` DATE NOT NULL,
  `entry_opened_count` BIGINT NOT NULL DEFAULT 0,
  `intake_confirmed_count` BIGINT NOT NULL DEFAULT 0,
  `testee_created_count` BIGINT NOT NULL DEFAULT 0,
  `care_relationship_established_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_access_entry_daily` (`org_id`, `entry_id`, `stat_date`),
  KEY `idx_analytics_access_entry_date` (`org_id`, `stat_date`),
  KEY `idx_analytics_access_entry_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='接入漏斗入口日投影';

CREATE TABLE IF NOT EXISTS `analytics_assessment_service_org_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `stat_date` DATE NOT NULL,
  `answersheet_submitted_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_created_count` BIGINT NOT NULL DEFAULT 0,
  `report_generated_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_failed_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_assessment_service_org_daily` (`org_id`, `stat_date`),
  KEY `idx_analytics_assessment_service_org_date` (`org_id`, `stat_date`),
  KEY `idx_analytics_assessment_service_org_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='测评服务机构日投影';

CREATE TABLE IF NOT EXISTS `analytics_assessment_service_clinician_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `clinician_id` BIGINT UNSIGNED NOT NULL,
  `stat_date` DATE NOT NULL,
  `answersheet_submitted_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_created_count` BIGINT NOT NULL DEFAULT 0,
  `report_generated_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_failed_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_assessment_service_clinician_daily` (`org_id`, `clinician_id`, `stat_date`),
  KEY `idx_analytics_assessment_service_clinician_date` (`org_id`, `stat_date`),
  KEY `idx_analytics_assessment_service_clinician_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='测评服务从业者日投影';

CREATE TABLE IF NOT EXISTS `analytics_assessment_service_entry_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `entry_id` BIGINT UNSIGNED NOT NULL,
  `clinician_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `stat_date` DATE NOT NULL,
  `answersheet_submitted_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_created_count` BIGINT NOT NULL DEFAULT 0,
  `report_generated_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_failed_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_assessment_service_entry_daily` (`org_id`, `entry_id`, `stat_date`),
  KEY `idx_analytics_assessment_service_entry_date` (`org_id`, `stat_date`),
  KEY `idx_analytics_assessment_service_entry_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='测评服务入口日投影';

CREATE TABLE IF NOT EXISTS `analytics_assessment_service_content_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `content_type` VARCHAR(50) NOT NULL,
  `content_code` VARCHAR(100) NOT NULL,
  `stat_date` DATE NOT NULL,
  `answersheet_submitted_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_created_count` BIGINT NOT NULL DEFAULT 0,
  `report_generated_count` BIGINT NOT NULL DEFAULT 0,
  `assessment_failed_count` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_assessment_service_content_daily` (`org_id`, `content_type`, `content_code`, `stat_date`),
  KEY `idx_analytics_assessment_service_content_date` (`org_id`, `stat_date`),
  KEY `idx_analytics_assessment_service_content_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='测评服务内容日投影';

CREATE TABLE IF NOT EXISTS `analytics_plan_task_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL,
  `plan_id` BIGINT UNSIGNED NOT NULL,
  `stat_date` DATE NOT NULL,
  `task_created_count` BIGINT NOT NULL DEFAULT 0,
  `task_opened_count` BIGINT NOT NULL DEFAULT 0,
  `task_completed_count` BIGINT NOT NULL DEFAULT 0,
  `task_expired_count` BIGINT NOT NULL DEFAULT 0,
  `enrolled_testees` BIGINT NOT NULL DEFAULT 0,
  `active_testees` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_analytics_plan_task_daily` (`org_id`, `plan_id`, `stat_date`),
  KEY `idx_analytics_plan_task_org_date` (`org_id`, `stat_date`),
  KEY `idx_analytics_plan_task_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='计划任务日投影';
