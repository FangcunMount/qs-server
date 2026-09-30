-- The event-to-Assessment identity is needed for safe recovery and must not
-- disappear just because application code is rolled back.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'qs_rm_evaluation_request_ref rollback requires manual review';
