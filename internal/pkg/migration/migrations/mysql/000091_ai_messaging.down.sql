-- Operational rollback keeps this schema and all unconfirmed records.
-- Offline removal requires separate data-retention review; intentionally no DROP.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'MQ schema downgrade requires separate retained-data review';
