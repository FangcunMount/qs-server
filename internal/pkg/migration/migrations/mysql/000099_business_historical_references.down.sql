-- Historical conclusions survive the controlled service rollback.
-- Removing them requires a separately reviewed data-retention operation.
SIGNAL SQLSTATE '45000'
  SET MESSAGE_TEXT = 'historical reference migration has no automatic down; preserve retirement conclusions';
