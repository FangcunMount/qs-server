-- Business request data is preserved. Roll back applications without downgrading this index.
DROP TABLE ai_bridge_request_assessments;
ALTER TABLE ai_bridge_requests
 DROP INDEX ix_ai_runtime_org_time,
 DROP INDEX ix_ai_runtime_org_status,
 DROP INDEX ix_ai_runtime_org_testee,
 DROP COLUMN organization_id, DROP COLUMN subject_id, DROP COLUMN testee_id,
 DROP COLUMN created_at, DROP COLUMN updated_at;
