DROP TABLE IF EXISTS `event_delivery_dead_letter`;

ALTER TABLE `domain_event_outbox`
  DROP INDEX `idx_outbox_org_retry_due`,
  DROP COLUMN `manual_replay_request_id`,
  DROP COLUMN `last_error_kind`,
  DROP COLUMN `retry_disposition`,
  DROP COLUMN `org_id`;

ALTER TABLE `runtime_checkpoint`
  DROP INDEX `idx_runtime_checkpoint_retry_due`,
  DROP COLUMN `action_request_id`,
  DROP COLUMN `retry_event_id`,
  DROP COLUMN `retry_policy_version`,
  DROP COLUMN `policy_max_attempts`,
  DROP COLUMN `next_attempt_at`,
  DROP COLUMN `retry_disposition`,
  DROP COLUMN `attempt_origin`;
