ALTER TABLE `runtime_checkpoint`
  ADD COLUMN `attempt_origin` varchar(32) DEFAULT NULL COMMENT 'initial/automatic/manual/force/lease_recovery' AFTER `retryable`,
  ADD COLUMN `retry_disposition` varchar(32) DEFAULT NULL COMMENT 'automatic/manual_required/terminal' AFTER `attempt_origin`,
  ADD COLUMN `next_attempt_at` datetime(3) DEFAULT NULL COMMENT 'next authorized business attempt' AFTER `retry_disposition`,
  ADD COLUMN `policy_max_attempts` int unsigned DEFAULT NULL COMMENT 'automatic attempt budget snapshot' AFTER `next_attempt_at`,
  ADD COLUMN `retry_policy_version` varchar(64) DEFAULT NULL COMMENT 'retry policy snapshot version' AFTER `policy_max_attempts`,
  ADD COLUMN `retry_event_id` varchar(64) DEFAULT NULL COMMENT 'scheduled retry event id' AFTER `retry_policy_version`,
  ADD COLUMN `action_request_id` varchar(64) DEFAULT NULL COMMENT 'manual governance authorization request' AFTER `retry_event_id`,
  ADD KEY `idx_runtime_checkpoint_retry_due` (`scope`, `status`, `retry_disposition`, `next_attempt_at`);

UPDATE `runtime_checkpoint` AS rc
INNER JOIN (
  SELECT `assessment_id`, MAX(`attempt_no`) AS `latest_attempt`
  FROM `runtime_checkpoint`
  WHERE `scope` = 'evaluation_run' AND `deleted_at` IS NULL AND `assessment_id` IS NOT NULL
  GROUP BY `assessment_id`
) AS latest
  ON latest.`assessment_id` = rc.`assessment_id` AND latest.`latest_attempt` = rc.`attempt_no`
SET
  rc.`retry_disposition` = CASE
    WHEN rc.`retryable` = 0 THEN 'terminal'
    WHEN rc.`attempt_no` < 3 THEN 'automatic'
    ELSE 'manual_required'
  END,
  rc.`policy_max_attempts` = 3,
  rc.`retry_policy_version` = 'business-retry/v1'
WHERE rc.`scope` = 'evaluation_run' AND rc.`status` = 'failed' AND rc.`deleted_at` IS NULL;

ALTER TABLE `domain_event_outbox`
  ADD COLUMN `org_id` bigint DEFAULT NULL AFTER `aggregate_id`,
  ADD COLUMN `retry_disposition` varchar(32) DEFAULT NULL AFTER `attempt_count`,
  ADD COLUMN `last_error_kind` varchar(32) DEFAULT NULL AFTER `last_error`,
  ADD COLUMN `manual_replay_request_id` varchar(64) DEFAULT NULL AFTER `last_error_kind`,
  ADD KEY `idx_outbox_org_retry_due` (`org_id`, `status`, `retry_disposition`, `next_attempt_at`);

UPDATE `domain_event_outbox`
SET `org_id` = CAST(JSON_UNQUOTE(JSON_EXTRACT(`payload_json`, '$.data.org_id')) AS SIGNED)
WHERE JSON_VALID(`payload_json`) = 1
  AND JSON_EXTRACT(`payload_json`, '$.data.org_id') IS NOT NULL;

UPDATE `domain_event_outbox`
SET `retry_disposition` = CASE
  WHEN `attempt_count` < 30 THEN 'automatic'
  ELSE 'manual_required'
END
WHERE `status` = 'failed';

CREATE TABLE `event_delivery_dead_letter` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `message_id` varchar(128) NOT NULL,
  `event_id` varchar(64) DEFAULT NULL,
  `org_id` bigint DEFAULT NULL,
  `provider` varchar(32) NOT NULL,
  `topic_name` varchar(128) NOT NULL,
  `channel_name` varchar(128) NOT NULL,
  `delivery_attempts` int unsigned NOT NULL,
  `payload_json` longtext NOT NULL,
  `last_error` text NULL,
  `retry_disposition` varchar(32) NOT NULL DEFAULT 'manual_required',
  `replay_request_id` varchar(64) DEFAULT NULL,
  `replayed_at` datetime(3) DEFAULT NULL,
  `failed_at` datetime(3) NOT NULL,
  `created_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_delivery_dead_letter_identity` (`provider`, `topic_name`, `channel_name`, `message_id`),
  KEY `idx_delivery_dead_letter_org_disposition` (`org_id`, `retry_disposition`, `failed_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
