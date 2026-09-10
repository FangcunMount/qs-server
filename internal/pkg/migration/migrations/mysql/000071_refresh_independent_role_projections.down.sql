-- Projections are derived; after a coordinated IAM rollback, re-read its newer
-- policy version instead of copying stale local role lists back into IAM.
UPDATE staff SET authz_projection_pending=TRUE WHERE deleted_at IS NULL;
