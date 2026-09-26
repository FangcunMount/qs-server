-- Production recipient snapshots and suppression decisions must not be erased
-- by an application rollback. Retain the table and perform manual review.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'task_opened_reminder_batch rollback requires manual review';
