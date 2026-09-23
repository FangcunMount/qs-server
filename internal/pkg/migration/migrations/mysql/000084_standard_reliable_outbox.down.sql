-- New standard messages and their authorization ledger are not mock history.
-- An application rollback keeps this schema once any row has been written.
DROP PROCEDURE IF EXISTS guard_standard_reliable_outbox_rollback;
CREATE PROCEDURE guard_standard_reliable_outbox_rollback()
BEGIN
 IF EXISTS (SELECT 1 FROM rm_outbox LIMIT 1)
    OR EXISTS (SELECT 1 FROM qs_rm_replay_requests LIMIT 1)
    OR EXISTS (SELECT 1 FROM qs_rm_replay_items LIMIT 1) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'retain standard Outbox and replay ledger; roll back to a compatible application';
 END IF;
END;
CALL guard_standard_reliable_outbox_rollback();
DROP PROCEDURE guard_standard_reliable_outbox_rollback;
DROP TABLE qs_rm_replay_items;
DROP TABLE qs_rm_replay_requests;
DROP TABLE rm_outbox;
