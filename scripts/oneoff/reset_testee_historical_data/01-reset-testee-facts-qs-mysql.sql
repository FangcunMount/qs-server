-- Reset every Testee fact in one QS environment while preserving healthcare
-- master data and content/configuration data.
--
-- This script is intentionally NOT scoped by org_id. It must only be used when
-- the approved operation is to remove every Testee in the selected database.
-- Required session variables are documented in README.md.

SET @qs_old_autocommit = @@SESSION.autocommit;
SET @qs_delete_batch_size = COALESCE(@qs_delete_batch_size, 1000);
SET @qs_reset_resume = COALESCE(@qs_reset_resume, 0);

-- Fail before creating helper routines when the basic environment gate is absent.
SET @qs_basic_guard_sql = IF(
  COALESCE(@qs_reset_confirm, '') = 'DELETE_ALL_TESTEE_DATA'
  AND COALESCE(@qs_expected_database, '') <> ''
  AND CAST(DATABASE() AS BINARY) = CAST(@qs_expected_database AS BINARY)
  AND @qs_expected_testee_count IS NOT NULL
  AND @qs_expected_profile_count IS NOT NULL,
  'SELECT 1',
  'SELECT * FROM `_oneoff_refusing_qs_reset_check_confirmation_database_and_counts`'
);
PREPARE qs_basic_guard_stmt FROM @qs_basic_guard_sql;
EXECUTE qs_basic_guard_stmt;
DEALLOCATE PREPARE qs_basic_guard_stmt;

DROP TEMPORARY TABLE IF EXISTS `tmp_reset_required_qs_tables`;
CREATE TEMPORARY TABLE `tmp_reset_required_qs_tables` (
  `table_name` VARCHAR(128) NOT NULL PRIMARY KEY
) ENGINE=InnoDB;

INSERT INTO `tmp_reset_required_qs_tables` (`table_name`) VALUES
  ('assessment_entry_resolve_log'),
  ('assessment_entry_intake_log'),
  ('clinician_relation'),
  ('assessment_task'),
  ('plan_enrollment'),
  ('assessment_score'),
  ('evaluation_outcome'),
  ('assessment'),
  ('testee'),
  ('statistics_access_fact'),
  ('statistics_assessment_fact'),
  ('statistics_plan_fact'),
  ('statistics_access_daily'),
  ('statistics_assessment_daily'),
  ('statistics_plan_activity_daily'),
  ('statistics_plan_fulfillment_daily'),
  ('statistics_org_snapshot'),
  ('statistics_sync_run'),
  ('runtime_checkpoint'),
  ('retry_event_hold'),
  ('event_delivery_dead_letter'),
  ('domain_event_outbox'),
  ('seed_backfill_stage_attempt'),
  ('seed_backfill_stage'),
  ('seed_backfill_rollback_phase_attempt'),
  ('seed_backfill_rollback_resource'),
  ('seed_backfill_rollback_operation'),
  ('staff'),
  ('clinician'),
  ('assessment_entry'),
  ('assessment_plan');

DROP PROCEDURE IF EXISTS `_oneoff_assert_qs_reset_preflight`;
DROP PROCEDURE IF EXISTS `_oneoff_delete_batches`;
DROP PROCEDURE IF EXISTS `_oneoff_assert_qs_reset_postcheck`;

DELIMITER //

CREATE PROCEDURE `_oneoff_assert_qs_reset_preflight`()
BEGIN
  DECLARE v_missing_tables TEXT DEFAULT NULL;
  DECLARE v_actual BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_running BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_lock_acquired INT DEFAULT 0;
  DECLARE v_error_message TEXT DEFAULT NULL;

  IF COALESCE(@qs_reset_confirm, '') <> 'DELETE_ALL_TESTEE_DATA' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: set @qs_reset_confirm=DELETE_ALL_TESTEE_DATA';
  END IF;
  IF COALESCE(@qs_expected_database, '') = '' OR CAST(DATABASE() AS BINARY) <> CAST(@qs_expected_database AS BINARY) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: DATABASE() does not match @qs_expected_database';
  END IF;
  IF @qs_expected_testee_count IS NULL OR @qs_expected_profile_count IS NULL THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: expected Testee/Profile counts are required';
  END IF;
  IF @qs_delete_batch_size < 100 OR @qs_delete_batch_size > 10000 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: @qs_delete_batch_size must be between 100 and 10000';
  END IF;
  IF @qs_reset_resume NOT IN (0, 1) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: @qs_reset_resume must be 0 or 1';
  END IF;

  SELECT GROUP_CONCAT(r.table_name ORDER BY r.table_name SEPARATOR ',')
    INTO v_missing_tables
  FROM `tmp_reset_required_qs_tables` r
  LEFT JOIN information_schema.tables t
    ON t.table_schema = DATABASE() AND t.table_name = r.table_name
  WHERE t.table_name IS NULL;
  IF v_missing_tables IS NOT NULL THEN
    SET v_error_message = LEFT(CONCAT('refusing QS reset: missing tables: ', v_missing_tables), 128);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = v_error_message;
  END IF;

  SELECT COUNT(*) INTO v_actual FROM `testee`;
  IF @qs_reset_resume = 0 AND v_actual <> CAST(@qs_expected_testee_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: Testee count changed after export/preflight';
  END IF;
  IF @qs_reset_resume = 1 AND v_actual > CAST(@qs_expected_testee_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: resume Testee count exceeds original scope';
  END IF;
  SELECT COUNT(DISTINCT `profile_id`) INTO v_actual FROM `testee` WHERE `profile_id` IS NOT NULL;
  IF @qs_reset_resume = 0 AND v_actual <> CAST(@qs_expected_profile_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: Profile ID count changed after export/preflight';
  END IF;
  IF @qs_reset_resume = 1 AND v_actual > CAST(@qs_expected_profile_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: resume Profile ID count exceeds original scope';
  END IF;

  SELECT COUNT(*) INTO v_running
  FROM `statistics_sync_run`
  WHERE `status` IN ('running', 'pending');
  IF v_running <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: Statistics sync is still running';
  END IF;
  SELECT COUNT(*) INTO v_running
  FROM `seed_backfill_rollback_operation`
  WHERE `status` = 'running';
  IF v_running <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: a historical rollback operation is still running';
  END IF;
  SELECT COUNT(*) INTO v_running
  FROM `domain_event_outbox`
  WHERE `event_type` IN
        ('answersheet.submitted','evaluation.requested','evaluation.retry.requested','evaluation.outcome.committed','evaluation.failed',
         'interpretation.report.generated','interpretation.report.failed','interpretation.retry.requested',
         'task.opened','task.completed','task.expired','task.canceled')
    AND `status` <> 'published';
  IF v_running <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: Testee MySQL Outbox has unfinished rows';
  END IF;

  SELECT GET_LOCK(CONCAT('qs-reset-all-testees:', DATABASE()), 0) INTO v_lock_acquired;
  IF v_lock_acquired <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing QS reset: another reset session owns the database lock';
  END IF;
END//

CREATE PROCEDURE `_oneoff_delete_batches`(
  IN p_table_name VARCHAR(128),
  IN p_predicate TEXT,
  IN p_batch_size INT
)
BEGIN
  DECLARE v_rows BIGINT DEFAULT 0;
  DECLARE v_total BIGINT DEFAULT 0;

  IF p_table_name IS NULL OR p_table_name NOT REGEXP '^[a-z0-9_]+$' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'invalid table identifier passed to _oneoff_delete_batches';
  END IF;
  IF p_predicate IS NULL OR TRIM(p_predicate) = '' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'empty predicate passed to _oneoff_delete_batches';
  END IF;

  delete_loop: LOOP
    SET @qs_delete_sql = CONCAT(
      'DELETE FROM `', p_table_name, '` WHERE ', p_predicate,
      ' LIMIT ', CAST(p_batch_size AS CHAR)
    );
    PREPARE qs_delete_stmt FROM @qs_delete_sql;
    EXECUTE qs_delete_stmt;
    SET v_rows = ROW_COUNT();
    DEALLOCATE PREPARE qs_delete_stmt;
    COMMIT;
    SET v_total = v_total + v_rows;
    IF v_rows = 0 THEN
      LEAVE delete_loop;
    END IF;
  END LOOP;

  SELECT p_table_name AS resource, v_total AS rows_deleted;
END//

CREATE PROCEDURE `_oneoff_assert_qs_reset_postcheck`()
BEGIN
  DECLARE v_remaining BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_staff_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_clinician_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_entry_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_plan_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_staff_ids_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_clinician_ids_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_entry_ids_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_plan_ids_after BIGINT UNSIGNED DEFAULT 0;

  SELECT
      (SELECT COUNT(*) FROM `assessment_entry_resolve_log`)
    + (SELECT COUNT(*) FROM `assessment_entry_intake_log`)
    + (SELECT COUNT(*) FROM `clinician_relation`)
    + (SELECT COUNT(*) FROM `assessment_task`)
    + (SELECT COUNT(*) FROM `plan_enrollment`)
    + (SELECT COUNT(*) FROM `assessment_score`)
    + (SELECT COUNT(*) FROM `evaluation_outcome`)
    + (SELECT COUNT(*) FROM `assessment`)
    + (SELECT COUNT(*) FROM `testee`)
    + (SELECT COUNT(*) FROM `statistics_access_fact`)
    + (SELECT COUNT(*) FROM `statistics_assessment_fact`)
    + (SELECT COUNT(*) FROM `statistics_plan_fact`)
    + (SELECT COUNT(*) FROM `statistics_access_daily`)
    + (SELECT COUNT(*) FROM `statistics_assessment_daily`)
    + (SELECT COUNT(*) FROM `statistics_plan_activity_daily`)
    + (SELECT COUNT(*) FROM `statistics_plan_fulfillment_daily`)
    + (SELECT COUNT(*) FROM `statistics_org_snapshot`)
    + (SELECT COUNT(*) FROM `statistics_sync_run`)
    + (SELECT COUNT(*) FROM `runtime_checkpoint` WHERE `scope` = 'evaluation_run')
    + (SELECT COUNT(*) FROM `retry_event_hold` WHERE JSON_UNQUOTE(JSON_EXTRACT(`payload_json`, '$.eventType')) IN
        ('answersheet.submitted','evaluation.requested','evaluation.retry.requested','evaluation.outcome.committed','evaluation.failed',
         'interpretation.report.generated','interpretation.report.failed','interpretation.retry.requested',
         'task.opened','task.completed','task.expired','task.canceled'))
    + (SELECT COUNT(*) FROM `event_delivery_dead_letter` WHERE JSON_UNQUOTE(JSON_EXTRACT(`payload_json`, '$.eventType')) IN
        ('answersheet.submitted','evaluation.requested','evaluation.retry.requested','evaluation.outcome.committed','evaluation.failed',
         'interpretation.report.generated','interpretation.report.failed','interpretation.retry.requested',
         'task.opened','task.completed','task.expired','task.canceled'))
    + (SELECT COUNT(*) FROM `domain_event_outbox` WHERE `event_type` IN
        ('answersheet.submitted','evaluation.requested','evaluation.retry.requested','evaluation.outcome.committed','evaluation.failed',
         'interpretation.report.generated','interpretation.report.failed','interpretation.retry.requested',
         'task.opened','task.completed','task.expired','task.canceled'))
    + (SELECT COUNT(*) FROM `seed_backfill_stage_attempt`)
    + (SELECT COUNT(*) FROM `seed_backfill_stage`)
    + (SELECT COUNT(*) FROM `seed_backfill_rollback_phase_attempt`)
    + (SELECT COUNT(*) FROM `seed_backfill_rollback_resource`)
    + (SELECT COUNT(*) FROM `seed_backfill_rollback_operation`)
  INTO v_remaining;

  IF v_remaining <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'QS reset postcheck failed: Testee facts remain';
  END IF;

  SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO v_staff_after, v_staff_ids_after FROM `staff`;
  SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO v_clinician_after, v_clinician_ids_after FROM `clinician`;
  SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO v_entry_after, v_entry_ids_after FROM `assessment_entry`;
  SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO v_plan_after, v_plan_ids_after FROM `assessment_plan`;

  IF v_staff_after <> @qs_staff_before OR v_staff_ids_after <> @qs_staff_ids_before
     OR v_clinician_after <> @qs_clinician_before OR v_clinician_ids_after <> @qs_clinician_ids_before
     OR v_entry_after <> @qs_entry_before OR v_entry_ids_after <> @qs_entry_ids_before
     OR v_plan_after <> @qs_plan_before OR v_plan_ids_after <> @qs_plan_ids_before THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'QS reset postcheck failed: protected healthcare master data changed';
  END IF;
END//

DELIMITER ;

CALL `_oneoff_assert_qs_reset_preflight`();

SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO @qs_staff_before, @qs_staff_ids_before FROM `staff`;
SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO @qs_clinician_before, @qs_clinician_ids_before FROM `clinician`;
SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO @qs_entry_before, @qs_entry_ids_before FROM `assessment_entry`;
SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO @qs_plan_before, @qs_plan_ids_before FROM `assessment_plan`;

SELECT
  DATABASE() AS database_name,
  (SELECT COUNT(*) FROM `testee`) AS testee_count,
  (SELECT COUNT(DISTINCT `profile_id`) FROM `testee` WHERE `profile_id` IS NOT NULL) AS profile_id_count,
  @qs_staff_before AS protected_staff_count,
  @qs_clinician_before AS protected_clinician_count,
  @qs_entry_before AS protected_entry_count,
  @qs_plan_before AS protected_plan_count;

SET SESSION autocommit = 0;

-- Historical diagnostic/rollback control. FK children must be deleted first.
CALL `_oneoff_delete_batches`('seed_backfill_rollback_phase_attempt', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('seed_backfill_rollback_resource', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('seed_backfill_rollback_operation', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('seed_backfill_stage_attempt', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('seed_backfill_stage', '1=1', @qs_delete_batch_size);

-- Shared event/runtime tables are filtered to the current Testee pipeline.
CALL `_oneoff_delete_batches`('event_delivery_dead_letter',
  'JSON_UNQUOTE(JSON_EXTRACT(`payload_json`, ''$.eventType'')) IN
   (''answersheet.submitted'',''evaluation.requested'',''evaluation.retry.requested'',''evaluation.outcome.committed'',''evaluation.failed'',
    ''interpretation.report.generated'',''interpretation.report.failed'',''interpretation.retry.requested'',
    ''task.opened'',''task.completed'',''task.expired'',''task.canceled'')', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('retry_event_hold',
  'JSON_UNQUOTE(JSON_EXTRACT(`payload_json`, ''$.eventType'')) IN
   (''answersheet.submitted'',''evaluation.requested'',''evaluation.retry.requested'',''evaluation.outcome.committed'',''evaluation.failed'',
    ''interpretation.report.generated'',''interpretation.report.failed'',''interpretation.retry.requested'',
    ''task.opened'',''task.completed'',''task.expired'',''task.canceled'')', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('runtime_checkpoint', '`scope` = ''evaluation_run''', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('domain_event_outbox',
  '`event_type` IN
   (''answersheet.submitted'',''evaluation.requested'',''evaluation.retry.requested'',''evaluation.outcome.committed'',''evaluation.failed'',
    ''interpretation.report.generated'',''interpretation.report.failed'',''interpretation.retry.requested'',
    ''task.opened'',''task.completed'',''task.expired'',''task.canceled'')', @qs_delete_batch_size);

-- Statistics V2 is a rebuildable read side and is reset as a whole.
CALL `_oneoff_delete_batches`('statistics_org_snapshot', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('statistics_plan_fulfillment_daily', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('statistics_plan_activity_daily', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('statistics_assessment_daily', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('statistics_access_daily', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('statistics_plan_fact', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('statistics_assessment_fact', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('statistics_access_fact', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('statistics_sync_run', '1=1', @qs_delete_batch_size);

-- Testee-owned facts. Healthcare master tables and Entry/Plan definitions are untouched.
CALL `_oneoff_delete_batches`('assessment_entry_resolve_log', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('assessment_entry_intake_log', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('clinician_relation', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('assessment_task', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('plan_enrollment', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('assessment_score', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('evaluation_outcome', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('assessment', '1=1', @qs_delete_batch_size);
CALL `_oneoff_delete_batches`('testee', '1=1', @qs_delete_batch_size);

CALL `_oneoff_assert_qs_reset_postcheck`();

SELECT
  'completed' AS reset_status,
  DATABASE() AS database_name,
  (SELECT COUNT(*) FROM `testee`) AS remaining_testees,
  (SELECT COUNT(*) FROM `staff`) AS protected_staff_count,
  (SELECT COUNT(*) FROM `clinician`) AS protected_clinician_count,
  (SELECT COUNT(*) FROM `assessment_entry`) AS protected_entry_count,
  (SELECT COUNT(*) FROM `assessment_plan`) AS protected_plan_count;

SET SESSION autocommit = @qs_old_autocommit;
DO RELEASE_LOCK(CONCAT('qs-reset-all-testees:', DATABASE()));

DROP PROCEDURE IF EXISTS `_oneoff_assert_qs_reset_postcheck`;
DROP PROCEDURE IF EXISTS `_oneoff_delete_batches`;
DROP PROCEDURE IF EXISTS `_oneoff_assert_qs_reset_preflight`;
DROP TEMPORARY TABLE IF EXISTS `tmp_reset_required_qs_tables`;
