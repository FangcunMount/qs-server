-- Add evidence fields; immutable outcome result and payload schema are unchanged.
ALTER TABLE evaluation_outcome
 ADD COLUMN committed_event_id VARBINARY(128) NULL,
 ADD COLUMN committed_event_evidence JSON NULL,
 ADD UNIQUE KEY uq_evaluation_outcome_committed_event_id (committed_event_id);
