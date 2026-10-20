-- Explicit host installation. Sequence is original command-admission order,
-- not Broker delivery or model completion. Existing duplicates refuse DDL.
ALTER TABLE ai_messaging_operations
 ADD UNIQUE KEY uk_ai_messaging_command_sequence (aggregate_key,aggregate_sequence);
