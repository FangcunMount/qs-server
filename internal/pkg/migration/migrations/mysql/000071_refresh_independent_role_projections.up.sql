-- Deploy this final release after IAM independent-role apply. Display projections
-- are refreshed from IAM by the existing reconciliation runner, never inferred
-- from clinician identity or expanded from retired role aliases.
UPDATE staff SET authz_projection_pending=TRUE WHERE deleted_at IS NULL;
