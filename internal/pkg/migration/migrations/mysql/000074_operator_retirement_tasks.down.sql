-- Retirement audit cannot be dropped by an automatic application rollback.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Preserve operator retirement tasks; use an audited recovery procedure';
