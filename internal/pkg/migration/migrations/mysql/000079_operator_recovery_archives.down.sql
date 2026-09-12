-- Fail instead of discarding recovery evidence. Export and verify populated archives before structural rollback.
DROP PROCEDURE IF EXISTS guard_operator_recovery_archive_rollback;
CREATE PROCEDURE guard_operator_recovery_archive_rollback()
BEGIN
 IF EXISTS (SELECT 1 FROM operator_recovery_archives LIMIT 1) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'operator recovery archives must be preserved before rollback';
 END IF;
END;
CALL guard_operator_recovery_archive_rollback();
DROP PROCEDURE guard_operator_recovery_archive_rollback;
DROP TABLE operator_recovery_archives;
