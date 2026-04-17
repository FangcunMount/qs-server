ALTER TABLE `behavior_footprint`
    DROP INDEX `idx_bf_org_testee_event_del_time`,
    DROP INDEX `idx_bf_org_entry_event_del_time`,
    DROP INDEX `idx_bf_org_answersheet_event_del`,
    DROP INDEX `idx_bf_org_assessment_event_del`,
    ADD INDEX `idx_behavior_footprint_org_time` (`org_id`, `event_name`, `occurred_at`),
    ADD INDEX `idx_behavior_footprint_entry` (`entry_id`),
    ADD INDEX `idx_behavior_footprint_clinician` (`clinician_id`),
    ADD INDEX `idx_behavior_footprint_testee` (`testee_id`),
    ADD INDEX `idx_behavior_footprint_answersheet` (`answersheet_id`),
    ADD INDEX `idx_behavior_footprint_assessment` (`assessment_id`),
    ADD INDEX `idx_behavior_footprint_report` (`report_id`),
    ADD INDEX `idx_behavior_footprint_deleted_at` (`deleted_at`);

ALTER TABLE `assessment_episode`
    DROP INDEX `idx_ae_org_testee_del_submitted`,
    DROP INDEX `idx_ae_org_assessment_del`,
    ADD INDEX `idx_assessment_episode_org_testee` (`org_id`, `testee_id`),
    ADD INDEX `idx_assessment_episode_entry` (`entry_id`),
    ADD INDEX `idx_assessment_episode_clinician` (`clinician_id`),
    ADD INDEX `idx_assessment_episode_assessment` (`assessment_id`),
    ADD INDEX `idx_assessment_episode_report` (`report_id`),
    ADD INDEX `idx_assessment_episode_deleted_at` (`deleted_at`);
