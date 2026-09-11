-- Intentionally refuse destructive downgrade: removing invalidation can revive transferred QR codes.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'Store configuration rollback requires a compatible binary; preserve store and entry invalidation history';
