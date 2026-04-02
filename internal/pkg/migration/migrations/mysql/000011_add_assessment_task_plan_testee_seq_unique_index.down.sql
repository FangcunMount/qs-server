ALTER TABLE `assessment_task`
    DROP INDEX `uk_plan_testee_seq`,
    ADD INDEX `idx_plan_testee_seq` (`plan_id`, `testee_id`, `seq`);
