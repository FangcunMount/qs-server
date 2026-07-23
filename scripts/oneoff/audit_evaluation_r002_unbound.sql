-- EV-R002 read-only inventory of historical Assessments without a complete model binding.
-- Run against the Assessment MySQL database before choosing an archive or rebuild action.
-- This script performs no writes.

SELECT
  status,
  COUNT(*) AS assessment_count,
  MIN(created_at) AS oldest_created_at,
  MAX(created_at) AS newest_created_at
FROM assessment
WHERE deleted_at IS NULL
  AND (
    evaluation_model_kind IS NULL OR evaluation_model_kind = ''
    OR evaluation_model_code IS NULL OR evaluation_model_code = ''
    OR evaluation_model_version IS NULL OR evaluation_model_version = ''
  )
GROUP BY status
ORDER BY status;

SELECT
  id,
  org_id,
  testee_id,
  answer_sheet_id,
  questionnaire_code,
  questionnaire_version,
  status,
  origin_type,
  origin_id,
  created_at,
  updated_at
FROM assessment
WHERE deleted_at IS NULL
  AND (
    evaluation_model_kind IS NULL OR evaluation_model_kind = ''
    OR evaluation_model_code IS NULL OR evaluation_model_code = ''
    OR evaluation_model_version IS NULL OR evaluation_model_version = ''
  )
ORDER BY id
LIMIT 1000;
