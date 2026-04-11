ALTER TABLE `assessment_task`
    ADD INDEX `idx_task_process_plan_status_time` (`org_id`, `plan_id`, `status`, `deleted_at`, `planned_at`, `id`),
    ADD INDEX `idx_task_process_plan_testee_status_time` (`org_id`, `plan_id`, `testee_id`, `status`, `deleted_at`, `planned_at`, `id`);
