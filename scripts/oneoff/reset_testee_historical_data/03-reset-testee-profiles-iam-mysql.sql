-- Delete only IAM Profile/ProfileLink rows whose IDs were exported from QS
-- Testee rows. IAM User, LoginIdentity, Credential and TokenAudit rows remain.
--
-- Run this script from the qs-server repository root with --local-infile=1.
-- Required session variables and input files are documented in README.md.

SET @iam_old_autocommit = @@SESSION.autocommit;
SET @iam_delete_batch_size = COALESCE(@iam_delete_batch_size, 1000);
SET @iam_reset_resume = COALESCE(@iam_reset_resume, 0);

-- Fail before loading files or creating helper routines when the basic gate is absent.
SET @iam_basic_guard_sql = IF(
  COALESCE(@iam_reset_confirm, '') = 'DELETE_EXPORTED_TESTEE_PROFILES'
  AND COALESCE(@iam_expected_database, '') <> ''
  AND CAST(DATABASE() AS BINARY) = CAST(@iam_expected_database AS BINARY)
  AND @iam_expected_profile_count IS NOT NULL
  AND @iam_expected_healthcare_user_count IS NOT NULL,
  'SELECT 1',
  'SELECT * FROM `_oneoff_refusing_iam_reset_check_confirmation_database_and_counts`'
);
PREPARE iam_basic_guard_stmt FROM @iam_basic_guard_sql;
EXECUTE iam_basic_guard_stmt;
DEALLOCATE PREPARE iam_basic_guard_stmt;

DROP TEMPORARY TABLE IF EXISTS `tmp_reset_testee_profile_ids`;
CREATE TEMPORARY TABLE `tmp_reset_testee_profile_ids` (
  `profile_id` BIGINT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=InnoDB;

DROP TEMPORARY TABLE IF EXISTS `tmp_reset_healthcare_user_ids`;
CREATE TEMPORARY TABLE `tmp_reset_healthcare_user_ids` (
  `user_id` BIGINT UNSIGNED NOT NULL PRIMARY KEY
) ENGINE=InnoDB;

LOAD DATA LOCAL INFILE 'tmp/testee-data-reset/iam-profile-ids.tsv'
IGNORE INTO TABLE `tmp_reset_testee_profile_ids`
LINES TERMINATED BY '\n'
(@profile_id)
SET `profile_id` = CAST(TRIM(TRAILING '\r' FROM TRIM(@profile_id)) AS UNSIGNED);

LOAD DATA LOCAL INFILE 'tmp/testee-data-reset/iam-healthcare-user-ids.tsv'
IGNORE INTO TABLE `tmp_reset_healthcare_user_ids`
LINES TERMINATED BY '\n'
(@user_id)
SET `user_id` = CAST(TRIM(TRAILING '\r' FROM TRIM(@user_id)) AS UNSIGNED);

DROP TEMPORARY TABLE IF EXISTS `tmp_reset_healthcare_profile_conflicts`;
CREATE TEMPORARY TABLE `tmp_reset_healthcare_profile_conflicts` (
  `profile_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (`profile_id`, `user_id`)
) ENGINE=InnoDB;

INSERT IGNORE INTO `tmp_reset_healthcare_profile_conflicts` (`profile_id`, `user_id`)
SELECT l.`profile_id`, l.`user_id`
FROM `profile_links` l
JOIN `tmp_reset_testee_profile_ids` p ON p.`profile_id` = l.`profile_id`
JOIN `tmp_reset_healthcare_user_ids` h ON h.`user_id` = l.`user_id`;

DROP PROCEDURE IF EXISTS `_oneoff_assert_iam_reset_preflight`;
DROP PROCEDURE IF EXISTS `_oneoff_delete_iam_profile_batches`;
DROP PROCEDURE IF EXISTS `_oneoff_assert_iam_reset_postcheck`;

DELIMITER //

CREATE PROCEDURE `_oneoff_assert_iam_reset_preflight`()
BEGIN
  DECLARE v_actual BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_existing_profiles BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_missing_tables TEXT DEFAULT NULL;
  DECLARE v_lock_acquired INT DEFAULT 0;

  IF COALESCE(@iam_reset_confirm, '') <> 'DELETE_EXPORTED_TESTEE_PROFILES' THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: set @iam_reset_confirm=DELETE_EXPORTED_TESTEE_PROFILES';
  END IF;
  IF COALESCE(@iam_expected_database, '') = '' OR CAST(DATABASE() AS BINARY) <> CAST(@iam_expected_database AS BINARY) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: DATABASE() does not match @iam_expected_database';
  END IF;
  IF @iam_expected_profile_count IS NULL OR @iam_expected_healthcare_user_count IS NULL THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: expected Profile and healthcare User counts are required';
  END IF;
  IF @iam_delete_batch_size < 100 OR @iam_delete_batch_size > 10000 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: @iam_delete_batch_size must be between 100 and 10000';
  END IF;
  IF @iam_reset_resume NOT IN (0, 1) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: @iam_reset_resume must be 0 or 1';
  END IF;

  SELECT GROUP_CONCAT(required.table_name ORDER BY required.table_name SEPARATOR ',')
    INTO v_missing_tables
  FROM (
    SELECT 'users' AS table_name
    UNION ALL SELECT 'profiles'
    UNION ALL SELECT 'profile_links'
    UNION ALL SELECT 'auth_login_identities'
    UNION ALL SELECT 'auth_credentials'
    UNION ALL SELECT 'auth_token_audit'
  ) required
  LEFT JOIN information_schema.tables actual
    ON actual.table_schema = DATABASE() AND actual.table_name = required.table_name
  WHERE actual.table_name IS NULL;
  IF v_missing_tables IS NOT NULL THEN
    SET @iam_preflight_error = LEFT(CONCAT('refusing IAM reset: missing tables: ', v_missing_tables), 128);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = @iam_preflight_error;
  END IF;

  SELECT COUNT(*) INTO v_actual FROM `tmp_reset_testee_profile_ids`;
  IF v_actual <> CAST(@iam_expected_profile_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: exported Profile ID count does not match @iam_expected_profile_count';
  END IF;
  SELECT COUNT(*) INTO v_actual FROM `tmp_reset_healthcare_user_ids`;
  IF v_actual <> CAST(@iam_expected_healthcare_user_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: healthcare User ID count does not match @iam_expected_healthcare_user_count';
  END IF;
  SELECT COUNT(*) INTO v_actual FROM `tmp_reset_testee_profile_ids` WHERE `profile_id` = 0;
  IF v_actual <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: Profile export contains zero or a malformed ID';
  END IF;
  SELECT COUNT(*) INTO v_actual FROM `tmp_reset_healthcare_user_ids` WHERE `user_id` = 0;
  IF v_actual <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: healthcare User export contains zero or a malformed ID';
  END IF;
  SELECT COUNT(*) INTO v_actual
  FROM `users` u
  JOIN `tmp_reset_healthcare_user_ids` healthcare ON healthcare.`user_id` = u.`id`;
  IF v_actual <> CAST(@iam_expected_healthcare_user_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: some QS healthcare User IDs do not exist in IAM';
  END IF;

  SELECT COUNT(*) INTO v_existing_profiles
  FROM `profiles` p
  JOIN `tmp_reset_testee_profile_ids` target ON target.`profile_id` = p.`id`;
  IF @iam_reset_resume = 0 AND v_existing_profiles <> CAST(@iam_expected_profile_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: some exported Profile IDs do not exist; inspect before first apply';
  END IF;
  IF @iam_reset_resume = 1 AND v_existing_profiles > CAST(@iam_expected_profile_count AS UNSIGNED) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: resume Profile count exceeds exported scope';
  END IF;

  SELECT COUNT(*) INTO v_actual FROM `tmp_reset_healthcare_profile_conflicts`;
  IF v_actual <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: exported Testee Profiles are linked to healthcare Users';
  END IF;

  SELECT GET_LOCK(CONCAT('iam-reset-testee-profiles:', DATABASE()), 0) INTO v_lock_acquired;
  IF v_lock_acquired <> 1 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'refusing IAM reset: another reset session owns the database lock';
  END IF;
END//

CREATE PROCEDURE `_oneoff_delete_iam_profile_batches`(
  IN p_table_name VARCHAR(128),
  IN p_id_column VARCHAR(128),
  IN p_batch_size INT
)
BEGIN
  DECLARE v_table_exists INT DEFAULT 0;
  DECLARE v_rows BIGINT DEFAULT 0;
  DECLARE v_total BIGINT DEFAULT 0;

  IF NOT (
    (p_table_name = 'profile_links' AND p_id_column = 'profile_id')
    OR (p_table_name = 'profiles' AND p_id_column = 'id')
    OR (p_table_name = 'guardianships' AND p_id_column = 'child_id')
    OR (p_table_name = 'children' AND p_id_column = 'id')
  ) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'invalid IAM target passed to _oneoff_delete_iam_profile_batches';
  END IF;

  SELECT COUNT(*) INTO v_table_exists
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = p_table_name;

  IF v_table_exists = 1 THEN
    delete_loop: LOOP
      SET @iam_delete_sql = CONCAT(
        'DELETE FROM `', p_table_name, '` WHERE `', p_id_column,
        '` IN (SELECT `profile_id` FROM `tmp_reset_testee_profile_ids`) LIMIT ',
        CAST(p_batch_size AS CHAR)
      );
      PREPARE iam_delete_stmt FROM @iam_delete_sql;
      EXECUTE iam_delete_stmt;
      SET v_rows = ROW_COUNT();
      DEALLOCATE PREPARE iam_delete_stmt;
      COMMIT;
      SET v_total = v_total + v_rows;
      IF v_rows = 0 THEN
        LEAVE delete_loop;
      END IF;
    END LOOP;
  END IF;

  SELECT p_table_name AS resource, v_table_exists AS table_existed, v_total AS rows_deleted;
END//

CREATE PROCEDURE `_oneoff_assert_iam_reset_postcheck`()
BEGIN
  DECLARE v_remaining BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_users_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_identities_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_credentials_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_tokens_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_user_ids_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_identity_ids_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_credential_ids_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_token_ids_after BIGINT UNSIGNED DEFAULT 0;
  DECLARE v_legacy_exists INT DEFAULT 0;

  SELECT COUNT(*) INTO v_remaining
  FROM `profile_links` l
  JOIN `tmp_reset_testee_profile_ids` target ON target.`profile_id` = l.`profile_id`;
  IF v_remaining <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'IAM reset postcheck failed: target ProfileLink rows remain';
  END IF;
  SELECT COUNT(*) INTO v_remaining
  FROM `profiles` p
  JOIN `tmp_reset_testee_profile_ids` target ON target.`profile_id` = p.`id`;
  IF v_remaining <> 0 THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'IAM reset postcheck failed: target Profile rows remain';
  END IF;

  SELECT COUNT(*) INTO v_legacy_exists
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'guardianships';
  IF v_legacy_exists = 1 THEN
    SET @iam_legacy_remaining = 0;
    SET @iam_legacy_count_sql =
      'SELECT COUNT(*) INTO @iam_legacy_remaining FROM `guardianships` WHERE `child_id` IN (SELECT `profile_id` FROM `tmp_reset_testee_profile_ids`)';
    PREPARE iam_legacy_count_stmt FROM @iam_legacy_count_sql;
    EXECUTE iam_legacy_count_stmt;
    DEALLOCATE PREPARE iam_legacy_count_stmt;
    IF COALESCE(@iam_legacy_remaining, 0) <> 0 THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'IAM reset postcheck failed: target legacy Guardianship rows remain';
    END IF;
  END IF;

  SELECT COUNT(*) INTO v_legacy_exists
  FROM information_schema.tables
  WHERE table_schema = DATABASE() AND table_name = 'children';
  IF v_legacy_exists = 1 THEN
    SET @iam_legacy_remaining = 0;
    SET @iam_legacy_count_sql =
      'SELECT COUNT(*) INTO @iam_legacy_remaining FROM `children` WHERE `id` IN (SELECT `profile_id` FROM `tmp_reset_testee_profile_ids`)';
    PREPARE iam_legacy_count_stmt FROM @iam_legacy_count_sql;
    EXECUTE iam_legacy_count_stmt;
    DEALLOCATE PREPARE iam_legacy_count_stmt;
    IF COALESCE(@iam_legacy_remaining, 0) <> 0 THEN
      SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'IAM reset postcheck failed: target legacy Child rows remain';
    END IF;
  END IF;

  SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO v_users_after, v_user_ids_after FROM `users`;
  SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO v_identities_after, v_identity_ids_after FROM `auth_login_identities`;
  SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO v_credentials_after, v_credential_ids_after FROM `auth_credentials`;
  SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO v_tokens_after, v_token_ids_after FROM `auth_token_audit`;

  IF v_users_after <> @iam_users_before OR v_user_ids_after <> @iam_user_ids_before
     OR v_identities_after <> @iam_identities_before OR v_identity_ids_after <> @iam_identity_ids_before
     OR v_credentials_after <> @iam_credentials_before OR v_credential_ids_after <> @iam_credential_ids_before
     OR v_tokens_after <> @iam_tokens_before OR v_token_ids_after <> @iam_token_ids_before THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'IAM reset postcheck failed: protected identity/authentication data changed';
  END IF;
END//

DELIMITER ;

SELECT `profile_id`, `user_id`
FROM `tmp_reset_healthcare_profile_conflicts`
ORDER BY `profile_id`, `user_id`;

CALL `_oneoff_assert_iam_reset_preflight`();

SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO @iam_users_before, @iam_user_ids_before FROM `users`;
SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO @iam_identities_before, @iam_identity_ids_before FROM `auth_login_identities`;
SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO @iam_credentials_before, @iam_credential_ids_before FROM `auth_credentials`;
SELECT COUNT(*), COALESCE(BIT_XOR(`id`), 0) INTO @iam_tokens_before, @iam_token_ids_before FROM `auth_token_audit`;

SELECT
  DATABASE() AS database_name,
  (SELECT COUNT(*) FROM `tmp_reset_testee_profile_ids`) AS exported_profile_ids,
  (SELECT COUNT(*) FROM `tmp_reset_healthcare_user_ids`) AS protected_healthcare_users,
  (SELECT COUNT(*) FROM `tmp_reset_healthcare_profile_conflicts`) AS healthcare_profile_conflicts,
  @iam_users_before AS protected_user_count,
  @iam_identities_before AS protected_login_identity_count;

SET SESSION autocommit = 0;

CALL `_oneoff_delete_iam_profile_batches`('profile_links', 'profile_id', @iam_delete_batch_size);
CALL `_oneoff_delete_iam_profile_batches`('guardianships', 'child_id', @iam_delete_batch_size);
CALL `_oneoff_delete_iam_profile_batches`('profiles', 'id', @iam_delete_batch_size);
CALL `_oneoff_delete_iam_profile_batches`('children', 'id', @iam_delete_batch_size);

CALL `_oneoff_assert_iam_reset_postcheck`();

SELECT
  'completed' AS reset_status,
  DATABASE() AS database_name,
  (SELECT COUNT(*) FROM `users`) AS protected_user_count,
  (SELECT COUNT(*) FROM `auth_login_identities`) AS protected_login_identity_count,
  (SELECT COUNT(*) FROM `auth_credentials`) AS protected_credential_count,
  (SELECT COUNT(*) FROM `auth_token_audit`) AS protected_token_audit_count;

SET SESSION autocommit = @iam_old_autocommit;
DO RELEASE_LOCK(CONCAT('iam-reset-testee-profiles:', DATABASE()));

DROP PROCEDURE IF EXISTS `_oneoff_assert_iam_reset_postcheck`;
DROP PROCEDURE IF EXISTS `_oneoff_delete_iam_profile_batches`;
DROP PROCEDURE IF EXISTS `_oneoff_assert_iam_reset_preflight`;
DROP TEMPORARY TABLE IF EXISTS `tmp_reset_healthcare_profile_conflicts`;
DROP TEMPORARY TABLE IF EXISTS `tmp_reset_healthcare_user_ids`;
DROP TEMPORARY TABLE IF EXISTS `tmp_reset_testee_profile_ids`;
