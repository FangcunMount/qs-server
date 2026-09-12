-- Never discard accepted start evidence when rolling back application code.
DROP PROCEDURE IF EXISTS guard_assessment_conducting_rollback;
CREATE PROCEDURE guard_assessment_conducting_rollback()
BEGIN
 IF EXISTS (SELECT 1 FROM assessment WHERE conducting_context IS NOT NULL LIMIT 1) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'retain assessment conducting evidence and compatible application';
 END IF;
END;
CALL guard_assessment_conducting_rollback();
DROP PROCEDURE guard_assessment_conducting_rollback;
ALTER TABLE assessment DROP COLUMN conducting_context;
