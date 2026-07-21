CREATE TABLE IF NOT EXISTS `plan_enrollment` (
  `id` BIGINT UNSIGNED NOT NULL PRIMARY KEY COMMENT '计划参与轮次ID',
  `org_id` BIGINT NOT NULL COMMENT '组织ID',
  `plan_id` BIGINT UNSIGNED NOT NULL COMMENT '计划ID',
  `testee_id` BIGINT UNSIGNED NOT NULL COMMENT '受试者ID',
  `round` INT UNSIGNED NOT NULL COMMENT '同一受试者参与同一计划的轮次',
  `start_date` DATE NOT NULL COMMENT '本轮计划起始业务日（Asia/Shanghai）',
  `status` VARCHAR(32) NOT NULL COMMENT 'active/closed/terminated',
  `joined_at` DATETIME(3) NOT NULL COMMENT '加入时间',
  `closed_at` DATETIME(3) NULL DEFAULT NULL COMMENT '全部任务自然终态时间',
  `terminated_at` DATETIME(3) NULL DEFAULT NULL COMMENT '显式终止时间',
  `terminated_reason` VARCHAR(500) NOT NULL DEFAULT '' COMMENT '显式终止原因',
  `record_origin` VARCHAR(32) NOT NULL DEFAULT 'native' COMMENT 'native/derived_legacy',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  `created_by` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `updated_by` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `deleted_by` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `version` INT UNSIGNED NOT NULL DEFAULT 1,
  `active_slot` TINYINT GENERATED ALWAYS AS (
    CASE WHEN `status` = 'active' AND `deleted_at` IS NULL THEN 1 ELSE NULL END
  ) STORED COMMENT '同一计划和受试者最多一个活动轮次',
  UNIQUE KEY `uk_plan_enrollment_round` (`org_id`, `plan_id`, `testee_id`, `round`),
  UNIQUE KEY `uk_plan_enrollment_active` (`org_id`, `plan_id`, `testee_id`, `active_slot`),
  KEY `idx_plan_enrollment_testee` (`org_id`, `testee_id`, `status`),
  KEY `idx_plan_enrollment_plan` (`org_id`, `plan_id`, `status`),
  KEY `idx_plan_enrollment_joined_at` (`org_id`, `joined_at`),
  KEY `idx_plan_enrollment_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='患者参与测评计划的持久化轮次';

-- 历史任务只能恢复为一个派生轮次。ID 复用该组最小任务 ID，避免迁移时依赖应用 ID 生成器。
INSERT INTO `plan_enrollment` (
  `id`, `org_id`, `plan_id`, `testee_id`, `round`, `start_date`, `status`,
  `joined_at`, `closed_at`, `terminated_at`, `terminated_reason`, `record_origin`,
  `created_at`, `updated_at`, `created_by`, `updated_by`, `version`
)
SELECT
  MIN(t.`id`),
  t.`org_id`,
  t.`plan_id`,
  t.`testee_id`,
  1,
  DATE(MIN(t.`planned_at`)),
  CASE
    WHEN SUM(CASE WHEN t.`deleted_at` IS NULL AND t.`status` IN ('pending', 'opened') THEN 1 ELSE 0 END) > 0 THEN 'active'
    ELSE 'closed'
  END,
  MIN(t.`created_at`),
  CASE
    WHEN SUM(CASE WHEN t.`deleted_at` IS NULL AND t.`status` IN ('pending', 'opened') THEN 1 ELSE 0 END) = 0 THEN MAX(t.`updated_at`)
    ELSE NULL
  END,
  NULL,
  '',
  'derived_legacy',
  MIN(t.`created_at`),
  MAX(t.`updated_at`),
  0,
  0,
  1
FROM `assessment_task` t
GROUP BY t.`org_id`, t.`plan_id`, t.`testee_id`;

ALTER TABLE `assessment_task`
  ADD COLUMN `enrollment_id` BIGINT UNSIGNED NULL DEFAULT NULL AFTER `plan_id`,
  ADD COLUMN `expired_at` DATETIME(3) NULL DEFAULT NULL AFTER `completed_at`,
  ADD COLUMN `canceled_at` DATETIME(3) NULL DEFAULT NULL AFTER `expired_at`;

UPDATE `assessment_task` t
JOIN `plan_enrollment` e
  ON e.`org_id` = t.`org_id`
 AND e.`plan_id` = t.`plan_id`
 AND e.`testee_id` = t.`testee_id`
 AND e.`round` = 1
SET t.`enrollment_id` = e.`id`
;

-- 迁移之后所有活动任务必须属于明确的参与轮次。
ALTER TABLE `assessment_task`
  MODIFY COLUMN `enrollment_id` BIGINT UNSIGNED NOT NULL,
  DROP INDEX `uk_plan_testee_seq`,
  ADD UNIQUE KEY `uk_enrollment_seq` (`enrollment_id`, `seq`),
  ADD KEY `idx_task_enrollment_status` (`enrollment_id`, `status`),
  ADD KEY `idx_task_org_terminal_time` (`org_id`, `status`, `completed_at`, `expired_at`, `canceled_at`);
