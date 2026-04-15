CREATE TABLE IF NOT EXISTS `domain_event_outbox` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `event_id` VARCHAR(64) NOT NULL,
    `event_type` VARCHAR(128) NOT NULL,
    `aggregate_type` VARCHAR(64) NOT NULL,
    `aggregate_id` VARCHAR(64) NOT NULL,
    `topic_name` VARCHAR(128) NOT NULL,
    `payload_json` LONGTEXT NOT NULL,
    `status` VARCHAR(32) NOT NULL,
    `attempt_count` INT UNSIGNED NOT NULL DEFAULT 0,
    `next_attempt_at` DATETIME(3) NOT NULL,
    `last_error` TEXT NULL,
    `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    `published_at` DATETIME(3) NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_event_id` (`event_id`),
    KEY `idx_status_next_attempt_at` (`status`, `next_attempt_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
