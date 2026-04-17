ALTER TABLE `behavior_footprint`
    DROP INDEX `idx_behavior_footprint_org_time`,
    DROP INDEX `idx_behavior_footprint_entry`,
    DROP INDEX `idx_behavior_footprint_clinician`,
    DROP INDEX `idx_behavior_footprint_testee`,
    DROP INDEX `idx_behavior_footprint_answersheet`,
    DROP INDEX `idx_behavior_footprint_assessment`,
    DROP INDEX `idx_behavior_footprint_report`,
    DROP INDEX `idx_behavior_footprint_deleted_at`,
    ADD INDEX `idx_bf_org_testee_event_del_time` (`org_id`, `testee_id`, `event_name`, `deleted_at`, `occurred_at`),
    ADD INDEX `idx_bf_org_entry_event_del_time` (`org_id`, `entry_id`, `event_name`, `deleted_at`, `occurred_at`),
    ADD INDEX `idx_bf_org_answersheet_event_del` (`org_id`, `answersheet_id`, `event_name`, `deleted_at`),
    ADD INDEX `idx_bf_org_assessment_event_del` (`org_id`, `assessment_id`, `event_name`, `deleted_at`);

ALTER TABLE `assessment_episode`
    DROP INDEX `idx_assessment_episode_org_testee`,
    DROP INDEX `idx_assessment_episode_entry`,
    DROP INDEX `idx_assessment_episode_clinician`,
    DROP INDEX `idx_assessment_episode_assessment`,
    DROP INDEX `idx_assessment_episode_report`,
    DROP INDEX `idx_assessment_episode_deleted_at`,
    ADD INDEX `idx_ae_org_testee_del_submitted` (`org_id`, `testee_id`, `deleted_at`, `submitted_at`),
    ADD INDEX `idx_ae_org_assessment_del` (`org_id`, `assessment_id`, `deleted_at`);
