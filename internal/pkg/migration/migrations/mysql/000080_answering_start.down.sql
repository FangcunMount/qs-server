-- Rollback is allowed only before any answering start has been accepted.
-- Once used, keep the compatible structure and immutable business records.
DROP PROCEDURE IF EXISTS guard_answering_start_rollback;
CREATE PROCEDURE guard_answering_start_rollback()
BEGIN
 IF EXISTS (SELECT 1 FROM answering_start LIMIT 1) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'answering_start contains business facts; retain compatible schema';
 END IF;
END;
CALL guard_answering_start_rollback();
DROP PROCEDURE guard_answering_start_rollback;
DROP TABLE answering_start;
