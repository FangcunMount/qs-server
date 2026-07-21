-- EV-R016 read-only evidence (P2).
-- No writes. Archive result sets into the Evaluation ledger evidence section.
-- Gate: only consider delayed requeue / processing receipt changes when these
-- queries show material active-claim contention pressure.
-- Prefer Prometheus worker NACK/redelivery metrics when available.

-- =============================================================================
-- EV-R016 SQL1: currently running evaluation_run claims vs lease remaining
-- =============================================================================
SELECT
    COUNT(*) AS running_claims,
    SUM(lease_expires_at IS NOT NULL AND lease_expires_at > NOW(6)) AS active_lease,
    SUM(lease_expires_at IS NOT NULL AND lease_expires_at <= NOW(6)) AS lease_expired,
    ROUND(AVG(TIMESTAMPDIFF(SECOND, NOW(6), lease_expires_at)), 2) AS avg_lease_remaining_s,
    ROUND(MIN(TIMESTAMPDIFF(SECOND, NOW(6), lease_expires_at)), 2) AS min_lease_remaining_s,
    ROUND(MAX(TIMESTAMPDIFF(SECOND, NOW(6), lease_expires_at)), 2) AS max_lease_remaining_s
FROM runtime_checkpoint
WHERE scope = 'evaluation_run'
  AND status = 'running'
  AND deleted_at IS NULL;

-- =============================================================================
-- EV-R016 SQL2: lease-recovery origins among terminal runs (pressure / reclaim)
-- =============================================================================
SELECT
    COALESCE(attempt_origin, 'unknown') AS attempt_origin,
    status,
    COUNT(*) AS runs
FROM runtime_checkpoint
WHERE scope = 'evaluation_run'
  AND deleted_at IS NULL
  AND finished_at IS NOT NULL
  AND finished_at >= DATE_SUB(NOW(6), INTERVAL 14 DAY)
GROUP BY COALESCE(attempt_origin, 'unknown'), status
ORDER BY runs DESC;

-- =============================================================================
-- EV-R016 SQL3: assessments with concurrent-looking attempt churn
-- (multiple attempts started within a short window — proxy for redelivery)
-- =============================================================================
SELECT
    assessment_id,
    COUNT(*) AS attempts_14d,
    SUM(status = 'failed') AS failed_attempts,
    SUM(COALESCE(attempt_origin, '') = 'lease_recovery') AS lease_recovery_attempts,
    MIN(started_at) AS first_started_at,
    MAX(COALESCE(finished_at, started_at)) AS last_activity_at
FROM runtime_checkpoint
WHERE scope = 'evaluation_run'
  AND assessment_id IS NOT NULL
  AND deleted_at IS NULL
  AND started_at >= DATE_SUB(NOW(6), INTERVAL 14 DAY)
GROUP BY assessment_id
HAVING attempts_14d >= 2
ORDER BY attempts_14d DESC, lease_recovery_attempts DESC
LIMIT 50;

-- =============================================================================
-- EV-R016 SQL4: concurrent running claim count (snapshot)
-- Pair with worker MQ NACK/redelivery counters; >1 is rare under normal load.
-- =============================================================================
SELECT COUNT(*) AS concurrent_running_claims
FROM runtime_checkpoint
WHERE scope = 'evaluation_run'
  AND status = 'running'
  AND deleted_at IS NULL;
