-- Statistics is V2-only from this migration forward. Runtime checkpoints from
-- the retired realtime projector are not business data and must not survive.
DELETE FROM `runtime_checkpoint` WHERE `scope` = 'analytics_projector';

DROP TABLE IF EXISTS `behavior_footprint`;
DROP TABLE IF EXISTS `assessment_episode`;
DROP TABLE IF EXISTS `analytics_pending_event`;
DROP TABLE IF EXISTS `statistics_journey_daily`;
DROP TABLE IF EXISTS `statistics_content_daily`;
DROP TABLE IF EXISTS `statistics_plan_daily`;
DROP TABLE IF EXISTS `statistics_org_snapshot`;

RENAME TABLE `statistics_v2_org_snapshot` TO `statistics_org_snapshot`;

-- Stable event-source scans use (org_id, deleted_at, event_time, id). Each
-- lifecycle event is queried independently; no multi-time-column OR remains.
ALTER TABLE `plan_enrollment`
  ADD KEY `idx_enrollment_collect_joined` (`org_id`, `deleted_at`, `joined_at`, `id`),
  ADD KEY `idx_enrollment_collect_closed` (`org_id`, `deleted_at`, `closed_at`, `id`),
  ADD KEY `idx_enrollment_collect_terminated` (`org_id`, `deleted_at`, `terminated_at`, `id`);

ALTER TABLE `assessment_task`
  ADD KEY `idx_task_collect_created` (`org_id`, `deleted_at`, `created_at`, `id`),
  ADD KEY `idx_task_collect_opened` (`org_id`, `deleted_at`, `open_at`, `id`),
  ADD KEY `idx_task_collect_completed` (`org_id`, `deleted_at`, `completed_at`, `id`),
  ADD KEY `idx_task_collect_expired` (`org_id`, `deleted_at`, `expired_at`, `id`),
  ADD KEY `idx_task_collect_canceled` (`org_id`, `deleted_at`, `canceled_at`, `id`);

ALTER TABLE `assessment`
  ADD KEY `idx_assessment_collect_created` (`org_id`, `deleted_at`, `created_at`, `id`),
  ADD KEY `idx_assessment_collect_failed` (`org_id`, `deleted_at`, `failed_at`, `id`);
