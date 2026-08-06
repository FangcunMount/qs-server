-- Settle six explicitly approved stale Statistics runs.
--
-- Safety contract:
--   1. Run only after a fresh read-only check proves there is no Statistics
--      rebuild process/container and the Redis Statistics lease is absent.
--   2. Run with autocommit enabled in an interactive MySQL 8 session. This
--      script starts a transaction but intentionally does not commit it.
--   3. Type COMMIT only when the final verdict is READY_TO_COMMIT and all six
--      rows in the post-check are failed with error_code=stale_run_reconciled.
--      Otherwise type ROLLBACK.
--   4. Do not delete Fact rows. A failed run may have left idempotent Facts,
--      and later attempts reuse them through their canonical unique keys.

SET @settled_at = CURRENT_TIMESTAMP(3);

START TRANSACTION;

-- Lock the exact audit rows so the eligibility check and update cannot race a
-- concurrent writer. The operator must confirm that exactly these six rows are
-- shown before continuing with the rest of the sourced script.
SELECT
  `id`, `org_id`, `batch_key`, `attempt`, `trigger_type`, `run_mode`,
  `status`, `stage`, `started_at`, `updated_at`, `data_committed_at`,
  `finished_at`, `cache_generation`, `cache_published_at`,
  `error_code`, `error_message`
FROM `statistics_sync_run`
WHERE `id` IN (
  631012088902332974,
  631034496552022574,
  631349010061341230,
  631362645156442670,
  631363406154183214,
  631444782798877230
)
ORDER BY `id`
FOR UPDATE;

SET @eligible_count = (
  SELECT COUNT(*)
  FROM `statistics_sync_run`
  WHERE `id` IN (
    631012088902332974,
    631034496552022574,
    631349010061341230,
    631362645156442670,
    631363406154183214,
    631444782798877230
  )
    AND `org_id` = 1
    AND `trigger_type` = 'manual'
    AND `run_mode` IN ('repair', 'validate')
    AND `status` = 'running'
    AND `stage` IN ('collecting_assessment', 'collecting_plan')
    AND `data_committed_at` IS NULL
    AND `finished_at` IS NULL
    AND `cache_generation` = 0
    AND `cache_published_at` IS NULL
    AND `cache_resume_count` = 0
    AND `error_code` = ''
    AND `error_message` = ''
    AND `updated_at` <= @settled_at - INTERVAL 30 MINUTE
);

-- Fail closed: when any target has drifted, @eligible_count is not six and
-- this statement updates zero rows.
UPDATE `statistics_sync_run`
SET
  `status` = 'failed',
  `finished_at` = @settled_at,
  `error_code` = 'stale_run_reconciled',
  `error_message` = 'Manually settled as stale after confirming no active process or lease, no committed data, and no resume requirement.'
WHERE `id` IN (
    631012088902332974,
    631034496552022574,
    631349010061341230,
    631362645156442670,
    631363406154183214,
    631444782798877230
  )
  AND @eligible_count = 6
  AND `org_id` = 1
  AND `trigger_type` = 'manual'
  AND `run_mode` IN ('repair', 'validate')
  AND `status` = 'running'
  AND `stage` IN ('collecting_assessment', 'collecting_plan')
  AND `data_committed_at` IS NULL
  AND `finished_at` IS NULL
  AND `cache_generation` = 0
  AND `cache_published_at` IS NULL
  AND `cache_resume_count` = 0
  AND `error_code` = ''
  AND `error_message` = ''
  AND `updated_at` <= @settled_at - INTERVAL 30 MINUTE;

SET @affected_rows = ROW_COUNT();

SELECT
  @eligible_count AS `eligible_rows`,
  @affected_rows AS `affected_rows`,
  @settled_at AS `settled_at`,
  CASE
    WHEN @eligible_count = 6 AND @affected_rows = 6
      THEN 'READY_TO_COMMIT'
    ELSE 'ROLLBACK_REQUIRED'
  END AS `verdict`;

SELECT
  `id`, `batch_key`, `attempt`, `run_mode`, `status`, `stage`,
  `data_committed_at`, `finished_at`, `error_code`, `error_message`
FROM `statistics_sync_run`
WHERE `id` IN (
  631012088902332974,
  631034496552022574,
  631349010061341230,
  631362645156442670,
  631363406154183214,
  631444782798877230
)
ORDER BY `id`;

-- Intentionally no COMMIT here.
-- READY_TO_COMMIT + exact post-check: type COMMIT;
-- Any other result or uncertainty: type ROLLBACK;
