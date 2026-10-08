-- Retired PKs are durable replay protection, not disposable migration data.
-- Never silently drop them or invent absent standard command facts on downgrade.
SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'AI command retirement requires reviewed forward recovery';
