-- Reminder responsibility is durable business evidence. Application rollback
-- leaves this table in place; dropping it requires a separately reviewed,
-- explicit data-disposition operation.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'task reminder ledger down migration requires manual review';
