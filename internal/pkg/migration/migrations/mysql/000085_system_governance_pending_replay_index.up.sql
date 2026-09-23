CREATE INDEX idx_system_governance_replay_pending
ON system_governance_action_runs (org_id, action_id, status, id);
