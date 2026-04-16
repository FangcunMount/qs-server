INSERT INTO `assessment_entry_intake_log` (
  `org_id`,
  `clinician_id`,
  `entry_id`,
  `testee_id`,
  `testee_created`,
  `assignment_created`,
  `intake_at`,
  `created_at`,
  `updated_at`
)
SELECT
  r.`org_id`,
  r.`clinician_id`,
  r.`source_id` AS `entry_id`,
  r.`testee_id`,
  CASE
    WHEN t.`created_at` IS NOT NULL AND ABS(TIMESTAMPDIFF(SECOND, t.`created_at`, r.`bound_at`)) <= 5 THEN 1
    ELSE 0
  END AS `testee_created`,
  CASE
    WHEN EXISTS (
      SELECT 1
      FROM `clinician_relation` ar
      WHERE ar.`org_id` = r.`org_id`
        AND ar.`clinician_id` = r.`clinician_id`
        AND ar.`testee_id` = r.`testee_id`
        AND ar.`source_type` = 'assessment_entry'
        AND ar.`source_id` = r.`source_id`
        AND ar.`relation_type` IN ('assigned', 'primary', 'attending', 'collaborator')
        AND ar.`deleted_at` IS NULL
        AND ABS(TIMESTAMPDIFF(SECOND, ar.`bound_at`, r.`bound_at`)) <= 5
    ) THEN 1
    ELSE 0
  END AS `assignment_created`,
  r.`bound_at` AS `intake_at`,
  r.`created_at`,
  r.`updated_at`
FROM `clinician_relation` r
LEFT JOIN `testee` t
  ON t.`id` = r.`testee_id`
 AND t.`org_id` = r.`org_id`
 AND t.`deleted_at` IS NULL
LEFT JOIN `assessment_entry_intake_log` l
  ON l.`org_id` = r.`org_id`
 AND l.`clinician_id` = r.`clinician_id`
 AND l.`entry_id` = r.`source_id`
 AND l.`testee_id` = r.`testee_id`
 AND l.`intake_at` = r.`bound_at`
 AND l.`deleted_at` IS NULL
WHERE r.`relation_type` = 'creator'
  AND r.`source_type` = 'assessment_entry'
  AND r.`source_id` IS NOT NULL
  AND r.`deleted_at` IS NULL
  AND l.`id` IS NULL;
