INSERT IGNORE INTO `behavior_footprint` (
  `id`, `org_id`, `subject_type`, `subject_id`, `actor_type`, `actor_id`,
  `entry_id`, `clinician_id`, `source_clinician_id`, `testee_id`,
  `answersheet_id`, `assessment_id`, `report_id`, `event_name`, `occurred_at`,
  `properties_json`, `created_at`, `updated_at`
)
SELECT
  CONCAT('legacy:entry_opened:', l.`id`),
  l.`org_id`,
  'assessment_entry',
  l.`entry_id`,
  'assessment_entry',
  l.`entry_id`,
  l.`entry_id`,
  l.`clinician_id`,
  0,
  0,
  0,
  0,
  0,
  'entry_opened',
  l.`resolved_at`,
  JSON_OBJECT('legacy_source', 'assessment_entry_resolve_log', 'legacy_id', l.`id`),
  NOW(3),
  NOW(3)
FROM `assessment_entry_resolve_log` l
WHERE l.`deleted_at` IS NULL;

INSERT IGNORE INTO `behavior_footprint` (
  `id`, `org_id`, `subject_type`, `subject_id`, `actor_type`, `actor_id`,
  `entry_id`, `clinician_id`, `source_clinician_id`, `testee_id`,
  `answersheet_id`, `assessment_id`, `report_id`, `event_name`, `occurred_at`,
  `properties_json`, `created_at`, `updated_at`
)
SELECT
  CONCAT('legacy:intake_confirmed:', l.`id`),
  l.`org_id`,
  'testee',
  l.`testee_id`,
  'clinician',
  l.`clinician_id`,
  l.`entry_id`,
  l.`clinician_id`,
  0,
  l.`testee_id`,
  0,
  0,
  0,
  'intake_confirmed',
  l.`intake_at`,
  JSON_OBJECT('legacy_source', 'assessment_entry_intake_log', 'legacy_id', l.`id`),
  NOW(3),
  NOW(3)
FROM `assessment_entry_intake_log` l
WHERE l.`deleted_at` IS NULL;

INSERT IGNORE INTO `behavior_footprint` (
  `id`, `org_id`, `subject_type`, `subject_id`, `actor_type`, `actor_id`,
  `entry_id`, `clinician_id`, `source_clinician_id`, `testee_id`,
  `answersheet_id`, `assessment_id`, `report_id`, `event_name`, `occurred_at`,
  `properties_json`, `created_at`, `updated_at`
)
SELECT
  CONCAT('legacy:testee_profile_created:', l.`id`),
  l.`org_id`,
  'testee',
  l.`testee_id`,
  'clinician',
  l.`clinician_id`,
  l.`entry_id`,
  l.`clinician_id`,
  0,
  l.`testee_id`,
  0,
  0,
  0,
  'testee_profile_created',
  l.`intake_at`,
  JSON_OBJECT('legacy_source', 'assessment_entry_intake_log', 'legacy_id', l.`id`),
  NOW(3),
  NOW(3)
FROM `assessment_entry_intake_log` l
WHERE l.`deleted_at` IS NULL
  AND l.`testee_created` = 1;

INSERT IGNORE INTO `behavior_footprint` (
  `id`, `org_id`, `subject_type`, `subject_id`, `actor_type`, `actor_id`,
  `entry_id`, `clinician_id`, `source_clinician_id`, `testee_id`,
  `answersheet_id`, `assessment_id`, `report_id`, `event_name`, `occurred_at`,
  `properties_json`, `created_at`, `updated_at`
)
SELECT
  CONCAT('legacy:care_relationship_established:', l.`id`),
  l.`org_id`,
  'testee',
  l.`testee_id`,
  'clinician',
  l.`clinician_id`,
  l.`entry_id`,
  l.`clinician_id`,
  0,
  l.`testee_id`,
  0,
  0,
  0,
  'care_relationship_established',
  l.`intake_at`,
  JSON_OBJECT('legacy_source', 'assessment_entry_intake_log', 'legacy_id', l.`id`),
  NOW(3),
  NOW(3)
FROM `assessment_entry_intake_log` l
WHERE l.`deleted_at` IS NULL
  AND l.`assignment_created` = 1;

INSERT INTO `assessment_episode` (
  `episode_id`, `org_id`, `entry_id`, `clinician_id`, `testee_id`,
  `answersheet_id`, `assessment_id`, `report_id`, `attributed_intake_at`, `submitted_at`,
  `assessment_created_at`, `report_generated_at`, `failed_at`, `status`, `failure_reason`,
  `created_at`, `updated_at`
)
SELECT
  a.`answer_sheet_id`,
  a.`org_id`,
  NULL,
  NULL,
  a.`testee_id`,
  a.`answer_sheet_id`,
  a.`id`,
  NULL,
  NULL,
  COALESCE(a.`submitted_at`, a.`created_at`),
  a.`created_at`,
  CASE WHEN a.`interpreted_at` IS NOT NULL THEN a.`interpreted_at` ELSE NULL END,
  a.`failed_at`,
  CASE
    WHEN a.`failed_at` IS NOT NULL OR a.`status` = 'failed' THEN 'failed'
    WHEN a.`interpreted_at` IS NOT NULL OR a.`status` = 'interpreted' THEN 'completed'
    ELSE 'active'
  END,
  COALESCE(a.`failure_reason`, ''),
  NOW(3),
  NOW(3)
FROM `assessment` a
WHERE a.`deleted_at` IS NULL
  AND a.`answer_sheet_id` <> 0
ON DUPLICATE KEY UPDATE
  `assessment_id` = VALUES(`assessment_id`),
  `attributed_intake_at` = VALUES(`attributed_intake_at`),
  `submitted_at` = VALUES(`submitted_at`),
  `assessment_created_at` = VALUES(`assessment_created_at`),
  `report_generated_at` = VALUES(`report_generated_at`),
  `failed_at` = VALUES(`failed_at`),
  `status` = VALUES(`status`),
  `failure_reason` = VALUES(`failure_reason`),
  `updated_at` = NOW(3);

UPDATE `assessment_episode` e
JOIN (
  SELECT ranked.`answersheet_id`, ranked.`entry_id`, ranked.`clinician_id`, ranked.`intake_at`
  FROM (
    SELECT
      a.`answer_sheet_id` AS `answersheet_id`,
      l.`entry_id`,
      l.`clinician_id`,
      l.`intake_at`,
      ROW_NUMBER() OVER (
        PARTITION BY a.`answer_sheet_id`
        ORDER BY l.`intake_at` DESC, l.`id` DESC
      ) AS `rn`
    FROM `assessment` a
    JOIN `assessment_entry_intake_log` l
      ON l.`org_id` = a.`org_id`
     AND l.`testee_id` = a.`testee_id`
     AND l.`deleted_at` IS NULL
     AND l.`intake_at` <= COALESCE(a.`submitted_at`, a.`created_at`)
     AND l.`intake_at` >= DATE_SUB(COALESCE(a.`submitted_at`, a.`created_at`), INTERVAL 30 DAY)
    WHERE a.`deleted_at` IS NULL
      AND a.`answer_sheet_id` <> 0
  ) ranked
  WHERE ranked.`rn` = 1
) matched
  ON matched.`answersheet_id` = e.`answersheet_id`
SET
  e.`entry_id` = COALESCE(e.`entry_id`, matched.`entry_id`),
  e.`clinician_id` = COALESCE(e.`clinician_id`, matched.`clinician_id`),
  e.`attributed_intake_at` = COALESCE(e.`attributed_intake_at`, matched.`intake_at`),
  e.`updated_at` = NOW(3)
WHERE e.`deleted_at` IS NULL;

INSERT IGNORE INTO `behavior_footprint` (
  `id`, `org_id`, `subject_type`, `subject_id`, `actor_type`, `actor_id`,
  `entry_id`, `clinician_id`, `source_clinician_id`, `testee_id`,
  `answersheet_id`, `assessment_id`, `report_id`, `event_name`, `occurred_at`,
  `properties_json`, `created_at`, `updated_at`
)
SELECT
  CONCAT('legacy:answersheet_submitted:', e.`answersheet_id`),
  e.`org_id`,
  'answersheet',
  e.`answersheet_id`,
  'testee',
  e.`testee_id`,
  COALESCE(e.`entry_id`, 0),
  COALESCE(e.`clinician_id`, 0),
  0,
  e.`testee_id`,
  e.`answersheet_id`,
  COALESCE(e.`assessment_id`, 0),
  COALESCE(e.`report_id`, 0),
  'answersheet_submitted',
  e.`submitted_at`,
  JSON_OBJECT('legacy_source', 'assessment', 'episode_id', e.`episode_id`),
  NOW(3),
  NOW(3)
FROM `assessment_episode` e
WHERE e.`deleted_at` IS NULL;

INSERT IGNORE INTO `behavior_footprint` (
  `id`, `org_id`, `subject_type`, `subject_id`, `actor_type`, `actor_id`,
  `entry_id`, `clinician_id`, `source_clinician_id`, `testee_id`,
  `answersheet_id`, `assessment_id`, `report_id`, `event_name`, `occurred_at`,
  `properties_json`, `created_at`, `updated_at`
)
SELECT
  CONCAT('legacy:assessment_created:', e.`assessment_id`),
  e.`org_id`,
  'assessment',
  e.`assessment_id`,
  'testee',
  e.`testee_id`,
  COALESCE(e.`entry_id`, 0),
  COALESCE(e.`clinician_id`, 0),
  0,
  e.`testee_id`,
  e.`answersheet_id`,
  e.`assessment_id`,
  COALESCE(e.`report_id`, 0),
  'assessment_created',
  e.`assessment_created_at`,
  JSON_OBJECT('legacy_source', 'assessment', 'episode_id', e.`episode_id`),
  NOW(3),
  NOW(3)
FROM `assessment_episode` e
WHERE e.`deleted_at` IS NULL
  AND e.`assessment_id` IS NOT NULL
  AND e.`assessment_created_at` IS NOT NULL;

INSERT IGNORE INTO `behavior_footprint` (
  `id`, `org_id`, `subject_type`, `subject_id`, `actor_type`, `actor_id`,
  `entry_id`, `clinician_id`, `source_clinician_id`, `testee_id`,
  `answersheet_id`, `assessment_id`, `report_id`, `event_name`, `occurred_at`,
  `properties_json`, `created_at`, `updated_at`
)
SELECT
  CONCAT('legacy:report_generated:', e.`assessment_id`),
  e.`org_id`,
  'assessment',
  e.`assessment_id`,
  'assessment',
  e.`assessment_id`,
  COALESCE(e.`entry_id`, 0),
  COALESCE(e.`clinician_id`, 0),
  0,
  e.`testee_id`,
  e.`answersheet_id`,
  e.`assessment_id`,
  COALESCE(e.`report_id`, 0),
  'report_generated',
  e.`report_generated_at`,
  JSON_OBJECT('legacy_source', 'assessment', 'episode_id', e.`episode_id`),
  NOW(3),
  NOW(3)
FROM `assessment_episode` e
WHERE e.`deleted_at` IS NULL
  AND e.`report_generated_at` IS NOT NULL;

INSERT INTO `analytics_projection_org_daily` (
  `org_id`, `stat_date`,
  `entry_opened_count`, `intake_confirmed_count`, `testee_profile_created_count`,
  `care_relationship_established_count`, `care_relationship_transferred_count`,
  `answersheet_submitted_count`, `assessment_created_count`, `report_generated_count`,
  `episode_completed_count`, `episode_failed_count`,
  `created_at`, `updated_at`
)
SELECT
  agg.`org_id`,
  agg.`stat_date`,
  SUM(agg.`entry_opened_count`),
  SUM(agg.`intake_confirmed_count`),
  SUM(agg.`testee_profile_created_count`),
  SUM(agg.`care_relationship_established_count`),
  SUM(agg.`care_relationship_transferred_count`),
  SUM(agg.`answersheet_submitted_count`),
  SUM(agg.`assessment_created_count`),
  SUM(agg.`report_generated_count`),
  SUM(agg.`episode_completed_count`),
  SUM(agg.`episode_failed_count`),
  NOW(3),
  NOW(3)
FROM (
  SELECT `org_id`, DATE(`occurred_at`) AS `stat_date`, 1 AS `entry_opened_count`, 0 AS `intake_confirmed_count`, 0 AS `testee_profile_created_count`, 0 AS `care_relationship_established_count`, 0 AS `care_relationship_transferred_count`, 0 AS `answersheet_submitted_count`, 0 AS `assessment_created_count`, 0 AS `report_generated_count`, 0 AS `episode_completed_count`, 0 AS `episode_failed_count`
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'entry_opened'
  UNION ALL
  SELECT `org_id`, DATE(`occurred_at`), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'intake_confirmed'
  UNION ALL
  SELECT `org_id`, DATE(`occurred_at`), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'testee_profile_created'
  UNION ALL
  SELECT `org_id`, DATE(`occurred_at`), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'care_relationship_established'
  UNION ALL
  SELECT `org_id`, DATE(`occurred_at`), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'care_relationship_transferred'
  UNION ALL
  SELECT `org_id`, DATE(`submitted_at`), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL
  UNION ALL
  SELECT `org_id`, DATE(`assessment_created_at`), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL AND `assessment_created_at` IS NOT NULL
  UNION ALL
  SELECT `org_id`, DATE(`report_generated_at`), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL AND `report_generated_at` IS NOT NULL
  UNION ALL
  SELECT `org_id`, DATE(`failed_at`), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM `assessment` WHERE `deleted_at` IS NULL AND `failed_at` IS NOT NULL
) agg
GROUP BY agg.`org_id`, agg.`stat_date`
ON DUPLICATE KEY UPDATE
  `entry_opened_count` = VALUES(`entry_opened_count`),
  `intake_confirmed_count` = VALUES(`intake_confirmed_count`),
  `testee_profile_created_count` = VALUES(`testee_profile_created_count`),
  `care_relationship_established_count` = VALUES(`care_relationship_established_count`),
  `care_relationship_transferred_count` = VALUES(`care_relationship_transferred_count`),
  `answersheet_submitted_count` = VALUES(`answersheet_submitted_count`),
  `assessment_created_count` = VALUES(`assessment_created_count`),
  `report_generated_count` = VALUES(`report_generated_count`),
  `episode_completed_count` = VALUES(`episode_completed_count`),
  `episode_failed_count` = VALUES(`episode_failed_count`),
  `updated_at` = NOW(3);

INSERT INTO `analytics_projection_clinician_daily` (
  `org_id`, `clinician_id`, `stat_date`,
  `entry_opened_count`, `intake_confirmed_count`, `testee_profile_created_count`,
  `care_relationship_established_count`, `care_relationship_transferred_count`,
  `answersheet_submitted_count`, `assessment_created_count`, `report_generated_count`,
  `episode_completed_count`, `episode_failed_count`,
  `created_at`, `updated_at`
)
SELECT
  agg.`org_id`,
  agg.`clinician_id`,
  agg.`stat_date`,
  SUM(agg.`entry_opened_count`),
  SUM(agg.`intake_confirmed_count`),
  SUM(agg.`testee_profile_created_count`),
  SUM(agg.`care_relationship_established_count`),
  SUM(agg.`care_relationship_transferred_count`),
  SUM(agg.`answersheet_submitted_count`),
  SUM(agg.`assessment_created_count`),
  SUM(agg.`report_generated_count`),
  SUM(agg.`episode_completed_count`),
  SUM(agg.`episode_failed_count`),
  NOW(3),
  NOW(3)
FROM (
  SELECT `org_id`, `clinician_id`, DATE(`occurred_at`) AS `stat_date`, 1 AS `entry_opened_count`, 0 AS `intake_confirmed_count`, 0 AS `testee_profile_created_count`, 0 AS `care_relationship_established_count`, 0 AS `care_relationship_transferred_count`, 0 AS `answersheet_submitted_count`, 0 AS `assessment_created_count`, 0 AS `report_generated_count`, 0 AS `episode_completed_count`, 0 AS `episode_failed_count`
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'entry_opened' AND `clinician_id` <> 0
  UNION ALL
  SELECT `org_id`, `clinician_id`, DATE(`occurred_at`), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'intake_confirmed' AND `clinician_id` <> 0
  UNION ALL
  SELECT `org_id`, `clinician_id`, DATE(`occurred_at`), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'testee_profile_created' AND `clinician_id` <> 0
  UNION ALL
  SELECT `org_id`, `clinician_id`, DATE(`occurred_at`), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'care_relationship_established' AND `clinician_id` <> 0
  UNION ALL
  SELECT `org_id`, `clinician_id`, DATE(`occurred_at`), 0, 0, 0, 0, 1, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'care_relationship_transferred' AND `clinician_id` <> 0
  UNION ALL
  SELECT `org_id`, `clinician_id`, DATE(`submitted_at`), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL AND `clinician_id` IS NOT NULL
  UNION ALL
  SELECT `org_id`, `clinician_id`, DATE(`assessment_created_at`), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL AND `clinician_id` IS NOT NULL AND `assessment_created_at` IS NOT NULL
  UNION ALL
  SELECT `org_id`, `clinician_id`, DATE(`report_generated_at`), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL AND `clinician_id` IS NOT NULL AND `report_generated_at` IS NOT NULL
  UNION ALL
  SELECT e.`org_id`, e.`clinician_id`, DATE(a.`failed_at`), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM `assessment_episode` e
  JOIN `assessment` a ON a.`answer_sheet_id` = e.`answersheet_id` AND a.`deleted_at` IS NULL
  WHERE e.`deleted_at` IS NULL AND e.`clinician_id` IS NOT NULL AND a.`failed_at` IS NOT NULL
) agg
GROUP BY agg.`org_id`, agg.`clinician_id`, agg.`stat_date`
ON DUPLICATE KEY UPDATE
  `entry_opened_count` = VALUES(`entry_opened_count`),
  `intake_confirmed_count` = VALUES(`intake_confirmed_count`),
  `testee_profile_created_count` = VALUES(`testee_profile_created_count`),
  `care_relationship_established_count` = VALUES(`care_relationship_established_count`),
  `care_relationship_transferred_count` = VALUES(`care_relationship_transferred_count`),
  `answersheet_submitted_count` = VALUES(`answersheet_submitted_count`),
  `assessment_created_count` = VALUES(`assessment_created_count`),
  `report_generated_count` = VALUES(`report_generated_count`),
  `episode_completed_count` = VALUES(`episode_completed_count`),
  `episode_failed_count` = VALUES(`episode_failed_count`),
  `updated_at` = NOW(3);

INSERT INTO `analytics_projection_entry_daily` (
  `org_id`, `entry_id`, `clinician_id`, `stat_date`,
  `entry_opened_count`, `intake_confirmed_count`, `testee_profile_created_count`,
  `care_relationship_established_count`, `care_relationship_transferred_count`,
  `answersheet_submitted_count`, `assessment_created_count`, `report_generated_count`,
  `episode_completed_count`, `episode_failed_count`,
  `created_at`, `updated_at`
)
SELECT
  agg.`org_id`,
  agg.`entry_id`,
  agg.`clinician_id`,
  agg.`stat_date`,
  SUM(agg.`entry_opened_count`),
  SUM(agg.`intake_confirmed_count`),
  SUM(agg.`testee_profile_created_count`),
  SUM(agg.`care_relationship_established_count`),
  SUM(agg.`care_relationship_transferred_count`),
  SUM(agg.`answersheet_submitted_count`),
  SUM(agg.`assessment_created_count`),
  SUM(agg.`report_generated_count`),
  SUM(agg.`episode_completed_count`),
  SUM(agg.`episode_failed_count`),
  NOW(3),
  NOW(3)
FROM (
  SELECT `org_id`, `entry_id`, `clinician_id`, DATE(`occurred_at`) AS `stat_date`, 1 AS `entry_opened_count`, 0 AS `intake_confirmed_count`, 0 AS `testee_profile_created_count`, 0 AS `care_relationship_established_count`, 0 AS `care_relationship_transferred_count`, 0 AS `answersheet_submitted_count`, 0 AS `assessment_created_count`, 0 AS `report_generated_count`, 0 AS `episode_completed_count`, 0 AS `episode_failed_count`
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'entry_opened' AND `entry_id` <> 0
  UNION ALL
  SELECT `org_id`, `entry_id`, `clinician_id`, DATE(`occurred_at`), 0, 1, 0, 0, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'intake_confirmed' AND `entry_id` <> 0
  UNION ALL
  SELECT `org_id`, `entry_id`, `clinician_id`, DATE(`occurred_at`), 0, 0, 1, 0, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'testee_profile_created' AND `entry_id` <> 0
  UNION ALL
  SELECT `org_id`, `entry_id`, `clinician_id`, DATE(`occurred_at`), 0, 0, 0, 1, 0, 0, 0, 0, 0, 0
  FROM `behavior_footprint` WHERE `deleted_at` IS NULL AND `event_name` = 'care_relationship_established' AND `entry_id` <> 0
  UNION ALL
  SELECT `org_id`, `entry_id`, `clinician_id`, DATE(`submitted_at`), 0, 0, 0, 0, 0, 1, 0, 0, 0, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL AND `entry_id` IS NOT NULL
  UNION ALL
  SELECT `org_id`, `entry_id`, `clinician_id`, DATE(`assessment_created_at`), 0, 0, 0, 0, 0, 0, 1, 0, 0, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL AND `entry_id` IS NOT NULL AND `assessment_created_at` IS NOT NULL
  UNION ALL
  SELECT `org_id`, `entry_id`, `clinician_id`, DATE(`report_generated_at`), 0, 0, 0, 0, 0, 0, 0, 1, 1, 0
  FROM `assessment_episode` WHERE `deleted_at` IS NULL AND `entry_id` IS NOT NULL AND `report_generated_at` IS NOT NULL
  UNION ALL
  SELECT e.`org_id`, e.`entry_id`, COALESCE(e.`clinician_id`, 0), DATE(a.`failed_at`), 0, 0, 0, 0, 0, 0, 0, 0, 0, 1
  FROM `assessment_episode` e
  JOIN `assessment` a ON a.`answer_sheet_id` = e.`answersheet_id` AND a.`deleted_at` IS NULL
  WHERE e.`deleted_at` IS NULL AND e.`entry_id` IS NOT NULL AND a.`failed_at` IS NOT NULL
) agg
GROUP BY agg.`org_id`, agg.`entry_id`, agg.`clinician_id`, agg.`stat_date`
ON DUPLICATE KEY UPDATE
  `clinician_id` = VALUES(`clinician_id`),
  `entry_opened_count` = VALUES(`entry_opened_count`),
  `intake_confirmed_count` = VALUES(`intake_confirmed_count`),
  `testee_profile_created_count` = VALUES(`testee_profile_created_count`),
  `care_relationship_established_count` = VALUES(`care_relationship_established_count`),
  `care_relationship_transferred_count` = VALUES(`care_relationship_transferred_count`),
  `answersheet_submitted_count` = VALUES(`answersheet_submitted_count`),
  `assessment_created_count` = VALUES(`assessment_created_count`),
  `report_generated_count` = VALUES(`report_generated_count`),
  `episode_completed_count` = VALUES(`episode_completed_count`),
  `episode_failed_count` = VALUES(`episode_failed_count`),
  `updated_at` = NOW(3);
