-- EV-R010 / EV-R012 read-only evidence (P1).
-- No writes. Archive result sets into the Evaluation ledger evidence section.
-- Run against target qs MySQL; review before any Lease heartbeat or decoder retirement.

-- =============================================================================
-- EV-R010: execution duration vs default 120s Lease (by model_kind)
-- =============================================================================
-- Approximate percentiles via sorted sample buckets; prefer Prometheus histograms
-- in production when available (qs_evaluation_* latency metrics).

SELECT
    COALESCE(eo.model_kind, 'unknown') AS model_kind,
    COUNT(*) AS succeeded_runs,
    ROUND(AVG(TIMESTAMPDIFF(MICROSECOND, rc.started_at, rc.finished_at)) / 1000, 2) AS avg_ms,
    ROUND(MIN(TIMESTAMPDIFF(MICROSECOND, rc.started_at, rc.finished_at)) / 1000, 2) AS min_ms,
    ROUND(MAX(TIMESTAMPDIFF(MICROSECOND, rc.started_at, rc.finished_at)) / 1000, 2) AS max_ms,
    SUM(TIMESTAMPDIFF(SECOND, rc.started_at, rc.finished_at) >= 60) AS ge_60s,
    SUM(TIMESTAMPDIFF(SECOND, rc.started_at, rc.finished_at) >= 100) AS ge_100s,
    SUM(TIMESTAMPDIFF(SECOND, rc.started_at, rc.finished_at) >= 120) AS ge_120s_lease
FROM runtime_checkpoint AS rc
LEFT JOIN evaluation_outcome AS eo
    ON eo.evaluation_run_id = rc.resource_id
WHERE rc.scope = 'evaluation_run'
  AND rc.status = 'succeeded'
  AND rc.finished_at IS NOT NULL
  AND rc.deleted_at IS NULL
GROUP BY COALESCE(eo.model_kind, 'unknown')
ORDER BY max_ms DESC;

-- Runs still running with lease past now (pressure signal for R010 heartbeat decision).
SELECT
    COUNT(*) AS running_lease_expired
FROM runtime_checkpoint
WHERE scope = 'evaluation_run'
  AND status = 'running'
  AND lease_expires_at IS NOT NULL
  AND lease_expires_at <= NOW(6)
  AND deleted_at IS NULL;

-- =============================================================================
-- EV-R012: Outcome schema_version + ReportInput presence
-- =============================================================================

SELECT
    schema_version,
    COUNT(*) AS outcome_count,
    SUM(report_input_json IS NULL OR report_input_json = '') AS missing_report_input,
    MIN(evaluated_at) AS earliest_evaluated_at,
    MAX(evaluated_at) AS latest_evaluated_at
FROM evaluation_outcome
GROUP BY schema_version
ORDER BY schema_version;

SELECT
    COALESCE(model_kind, 'unknown') AS model_kind,
    schema_version,
    COUNT(*) AS outcome_count,
    SUM(report_input_json IS NULL OR report_input_json = '') AS missing_report_input
FROM evaluation_outcome
GROUP BY COALESCE(model_kind, 'unknown'), schema_version
ORDER BY model_kind, schema_version;

-- Decoder / fallback candidates: schema < 2 still present.
SELECT
    schema_version,
    COUNT(*) AS legacy_outcome_count
FROM evaluation_outcome
WHERE schema_version < 2
GROUP BY schema_version
ORDER BY schema_version;
