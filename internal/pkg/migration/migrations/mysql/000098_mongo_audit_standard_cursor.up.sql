-- The existing operational checkpoint retains ordered BSON rm_outbox identities.
-- Application v2 restarts v1 cycles under revision CAS instead of reinterpreting
-- their legacy numeric aggregate cursor. No business ledger is introduced.
ALTER TABLE mongo_consistency_audit_checkpoint
  ADD COLUMN outbox_cursor VARBINARY(1024) NULL,
  ADD COLUMN outbox_upper_bound VARBINARY(1024) NULL;
