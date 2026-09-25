ALTER TABLE `event_delivery_dead_letter`
  ADD COLUMN `transport_message_id` varchar(64) NOT NULL DEFAULT '' AFTER `message_id`,
  DROP INDEX `uk_delivery_dead_letter_identity`,
  ADD UNIQUE KEY `uk_delivery_dead_letter_identity`
    (`provider`, `topic_name`, `channel_name`, `message_id`, `transport_message_id`),
  ALGORITHM=INPLACE,
  LOCK=NONE;
