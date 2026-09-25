-- Adding the legacy unique key first intentionally refuses rollback when
-- multiple physical deliveries share one logical message UUID.
ALTER TABLE `event_delivery_dead_letter`
  ADD UNIQUE KEY `uk_delivery_dead_letter_legacy_identity`
    (`provider`, `topic_name`, `channel_name`, `message_id`);

ALTER TABLE `event_delivery_dead_letter`
  DROP INDEX `uk_delivery_dead_letter_identity`,
  DROP COLUMN `transport_message_id`,
  RENAME INDEX `uk_delivery_dead_letter_legacy_identity` TO `uk_delivery_dead_letter_identity`;
