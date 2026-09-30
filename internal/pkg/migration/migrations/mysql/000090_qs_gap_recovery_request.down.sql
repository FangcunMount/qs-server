-- Preserve reviewed decisions across an application rollback. Never erase
-- recovery responsibility merely because the executable version changed.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'qs_rm_gap_recovery_request rollback requires manual review';
