CREATE TABLE `statistics_access_fact` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL, `fact_key` VARCHAR(255) NOT NULL, `core_hash` CHAR(64) NOT NULL, `fact_type` VARCHAR(64) NOT NULL,
  `occurred_at` DATETIME(3) NOT NULL, `stat_date` DATE NOT NULL,
  `source_type` VARCHAR(64) NOT NULL, `source_ref` VARCHAR(128) NOT NULL, `schema_version` INT UNSIGNED NOT NULL DEFAULT 1,
  `clinician_id` BIGINT UNSIGNED NULL, `source_clinician_id` BIGINT UNSIGNED NULL, `entry_id` BIGINT UNSIGNED NULL,
  `testee_id` BIGINT UNSIGNED NULL, `target_type` VARCHAR(32) NULL, `target_code` VARCHAR(100) NULL,
  `payload_json` JSON NULL, `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`), UNIQUE KEY `uk_statistics_access_fact_key` (`fact_key`),
  KEY `idx_statistics_access_fact_window` (`org_id`,`stat_date`,`fact_type`),
  KEY `idx_statistics_access_fact_source` (`source_type`,`source_ref`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `statistics_assessment_fact` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL, `fact_key` VARCHAR(255) NOT NULL, `core_hash` CHAR(64) NOT NULL, `fact_type` VARCHAR(64) NOT NULL,
  `occurred_at` DATETIME(3) NOT NULL, `stat_date` DATE NOT NULL,
  `source_type` VARCHAR(64) NOT NULL, `source_ref` VARCHAR(128) NOT NULL, `schema_version` INT UNSIGNED NOT NULL DEFAULT 1,
  `testee_id` BIGINT UNSIGNED NULL, `filler_id` BIGINT UNSIGNED NULL,
  `answersheet_id` BIGINT UNSIGNED NULL, `assessment_id` BIGINT UNSIGNED NULL, `outcome_id` BIGINT UNSIGNED NULL, `report_id` BIGINT UNSIGNED NULL,
  `questionnaire_code` VARCHAR(100) NULL, `questionnaire_version` VARCHAR(50) NULL,
  `model_kind` VARCHAR(50) NULL, `model_code` VARCHAR(100) NULL, `model_version` VARCHAR(50) NULL,
  `clinician_id` BIGINT UNSIGNED NULL, `entry_id` BIGINT UNSIGNED NULL,
  `origin_type` VARCHAR(32) NULL, `origin_id` VARCHAR(128) NULL,
  `plan_id` BIGINT UNSIGNED NULL, `enrollment_id` BIGINT UNSIGNED NULL, `task_id` BIGINT UNSIGNED NULL,
  `attribution_mode` VARCHAR(32) NULL, `payload_json` JSON NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`), UNIQUE KEY `uk_statistics_assessment_fact_key` (`fact_key`),
  KEY `idx_statistics_assessment_fact_window` (`org_id`,`stat_date`,`fact_type`),
  KEY `idx_statistics_assessment_fact_source` (`source_type`,`source_ref`),
  KEY `idx_statistics_assessment_fact_answersheet` (`answersheet_id`), KEY `idx_statistics_assessment_fact_assessment` (`assessment_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `statistics_plan_fact` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `org_id` BIGINT NOT NULL, `fact_key` VARCHAR(255) NOT NULL, `core_hash` CHAR(64) NOT NULL, `fact_type` VARCHAR(64) NOT NULL,
  `occurred_at` DATETIME(3) NOT NULL, `stat_date` DATE NOT NULL,
  `source_type` VARCHAR(64) NOT NULL, `source_ref` VARCHAR(128) NOT NULL, `schema_version` INT UNSIGNED NOT NULL DEFAULT 1,
  `plan_id` BIGINT UNSIGNED NOT NULL, `enrollment_id` BIGINT UNSIGNED NULL, `testee_id` BIGINT UNSIGNED NULL,
  `task_id` BIGINT UNSIGNED NULL, `task_seq` INT NULL, `scale_code` VARCHAR(100) NULL,
  `planned_at` DATETIME(3) NULL, `due_at` DATETIME(3) NULL, `completed_at` DATETIME(3) NULL, `task_status` VARCHAR(32) NULL,
  `payload_json` JSON NULL, `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`), UNIQUE KEY `uk_statistics_plan_fact_key` (`fact_key`),
  KEY `idx_statistics_plan_fact_window` (`org_id`,`stat_date`,`fact_type`),
  KEY `idx_statistics_plan_fact_source` (`source_type`,`source_ref`), KEY `idx_statistics_plan_fact_enrollment` (`enrollment_id`,`fact_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `statistics_access_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `org_id` BIGINT NOT NULL, `stat_date` DATE NOT NULL,
  `clinician_id` BIGINT UNSIGNED NOT NULL DEFAULT 0, `entry_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `entry_opened_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `intake_confirmed_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `testee_created_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `care_relationship_established_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `care_relationship_transferred_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`), UNIQUE KEY `uk_statistics_access_daily` (`org_id`,`stat_date`,`clinician_id`,`entry_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `statistics_assessment_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `org_id` BIGINT NOT NULL, `stat_date` DATE NOT NULL,
  `clinician_id` BIGINT UNSIGNED NOT NULL DEFAULT 0, `entry_id` BIGINT UNSIGNED NOT NULL DEFAULT 0, `origin_type` VARCHAR(32) NOT NULL DEFAULT '',
  `questionnaire_code` VARCHAR(100) NOT NULL DEFAULT '', `model_kind` VARCHAR(50) NOT NULL DEFAULT '', `model_code` VARCHAR(100) NOT NULL DEFAULT '',
  `answersheet_submitted_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `assessment_created_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `outcome_committed_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `assessment_failed_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `report_generated_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `report_failed_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3), PRIMARY KEY (`id`),
  UNIQUE KEY `uk_statistics_assessment_daily` (`org_id`,`stat_date`,`clinician_id`,`entry_id`,`origin_type`,`questionnaire_code`,`model_kind`,`model_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `statistics_plan_activity_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `org_id` BIGINT NOT NULL, `stat_date` DATE NOT NULL, `plan_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `enrollment_joined_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `enrollment_closed_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `enrollment_terminated_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `task_created_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `task_opened_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `task_completed_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `task_expired_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `task_canceled_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `participant_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`), UNIQUE KEY `uk_statistics_plan_activity_daily` (`org_id`,`stat_date`,`plan_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `statistics_plan_fulfillment_daily` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT, `org_id` BIGINT NOT NULL, `cohort_date` DATE NOT NULL, `plan_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `planned_task_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `planned_participant_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `due_task_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `completed_on_time_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `completed_overdue_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `uncompleted_overdue_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3), PRIMARY KEY (`id`),
  UNIQUE KEY `uk_statistics_plan_fulfillment_daily` (`org_id`,`cohort_date`,`plan_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `statistics_org_snapshot` (
  `org_id` BIGINT NOT NULL, `as_of_date` DATE NOT NULL, `snapshot_at` DATETIME(3) NOT NULL,
  `testee_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `clinician_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `active_clinician_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `entry_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `active_entry_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `active_enrollment_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `answersheet_submission_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `assessment_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `report_count` BIGINT UNSIGNED NOT NULL DEFAULT 0, `content_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3), PRIMARY KEY (`org_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `statistics_sync_run` (
  `id` BIGINT UNSIGNED NOT NULL, `org_id` BIGINT NOT NULL, `batch_key` VARCHAR(255) NOT NULL, `attempt` INT UNSIGNED NOT NULL,
  `trigger_type` VARCHAR(32) NOT NULL, `window_start` DATETIME(3) NOT NULL, `window_end` DATETIME(3) NOT NULL, `as_of_date` DATE NOT NULL,
  `status` VARCHAR(32) NOT NULL, `stage` VARCHAR(64) NOT NULL, `source_counts_json` JSON NULL, `fact_counts_json` JSON NULL, `result_counts_json` JSON NULL,
  `operator_id` BIGINT UNSIGNED NULL, `reason` VARCHAR(500) NOT NULL DEFAULT '', `started_at` DATETIME(3) NOT NULL,
  `data_committed_at` DATETIME(3) NULL, `finished_at` DATETIME(3) NULL, `error_code` VARCHAR(64) NOT NULL DEFAULT '', `error_message` VARCHAR(1000) NOT NULL DEFAULT '',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`), UNIQUE KEY `uk_statistics_sync_run_attempt` (`batch_key`,`attempt`),
  KEY `idx_statistics_sync_run_org_date_status` (`org_id`,`as_of_date`,`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
