-- Batch E0: Evaluation cleanup preflight (read-only).
-- Run against the target qs MySQL database before deleting legacy code or data.
-- Every result set should be reviewed and archived; this file performs no writes.

-- 1. Outcomes whose Assessment is missing or has not reached evaluated.
SELECT
    eo.id AS outcome_id,
    eo.assessment_id,
    eo.evaluation_run_id,
    a.status AS assessment_status
FROM evaluation_outcome AS eo
LEFT JOIN assessment AS a
    ON a.id = eo.assessment_id
   AND a.deleted_at IS NULL
WHERE a.id IS NULL
   OR a.status <> 'evaluated';

-- 2. Outcome/run contradictions. A durable outcome must point at a succeeded run.
SELECT
    eo.id AS outcome_id,
    eo.assessment_id,
    eo.evaluation_run_id,
    rc.status AS run_status,
    rc.attempt_no
FROM evaluation_outcome AS eo
LEFT JOIN runtime_checkpoint AS rc
    ON rc.scope = 'evaluation_run'
   AND rc.resource_id = eo.evaluation_run_id
   AND rc.deleted_at IS NULL
WHERE rc.id IS NULL
   OR rc.status <> 'succeeded';

-- 3. Compatibility pointer inventory. Batch E must not delete this column while
-- live rows still depend on it.
SELECT
    COUNT(*) AS assessments_with_current_run_id
FROM assessment
WHERE current_run_id IS NOT NULL
  AND deleted_at IS NULL;

-- 4. Persisted Outcome schema inventory. Unknown versions require an explicit
-- decoder or data migration before canonical model cleanup.
SELECT
    schema_version,
    COUNT(*) AS outcome_count,
    MIN(evaluated_at) AS earliest_evaluated_at,
    MAX(evaluated_at) AS latest_evaluated_at
FROM evaluation_outcome
GROUP BY schema_version
ORDER BY schema_version;

-- 5. Legacy scale payload inventory. Canonical scale outcomes carry dimensions;
-- historical payload-only rows must remain restorable during E2.
SELECT
    COUNT(*) AS legacy_scale_payload_count
FROM evaluation_outcome
WHERE model_kind = 'scale'
  AND JSON_VALID(payload_json)
  AND JSON_LENGTH(JSON_EXTRACT(payload_json, '$.dimensions')) = 0
  AND JSON_EXTRACT(payload_json, '$.detail.payload') IS NOT NULL;

-- 6. Legacy Assessment lifecycle inventory. Interpretation no longer advances
-- Assessment to interpreted; non-zero rows require a separate data migration.
SELECT
    COUNT(*) AS interpreted_assessment_count
FROM assessment
WHERE status = 'interpreted'
  AND deleted_at IS NULL;
