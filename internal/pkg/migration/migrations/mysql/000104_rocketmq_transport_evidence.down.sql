-- Retain every original failure envelope during application rollback.
SET @rmq_evidence_rows = (SELECT COUNT(*) FROM event_delivery_transport_evidence);
SET @rmq_evidence_down = IF(@rmq_evidence_rows=0,
  'DROP TABLE event_delivery_transport_evidence',
  'SELECT rocketmq_evidence_rollback_requires_preserved_records()');
PREPARE rmq_evidence_stmt FROM @rmq_evidence_down;
EXECUTE rmq_evidence_stmt;
DEALLOCATE PREPARE rmq_evidence_stmt;
