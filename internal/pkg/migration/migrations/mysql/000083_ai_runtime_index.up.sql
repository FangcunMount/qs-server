ALTER TABLE ai_bridge_requests
 ADD COLUMN organization_id BIGINT UNSIGNED NULL,
 ADD COLUMN subject_id VARCHAR(128) COLLATE utf8mb4_bin NULL,
 ADD COLUMN testee_id BIGINT UNSIGNED NULL,
 ADD COLUMN created_at DATETIME(6) NULL,
 ADD COLUMN updated_at DATETIME(6) NULL,
 ADD INDEX ix_ai_runtime_org_time (organization_id, created_at, request_id),
 ADD INDEX ix_ai_runtime_org_status (organization_id, status, created_at, request_id),
 ADD INDEX ix_ai_runtime_org_subject (organization_id, subject_id, created_at, request_id),
 ADD INDEX ix_ai_runtime_org_testee (organization_id, testee_id, created_at, request_id);

CREATE TABLE ai_bridge_request_assessments (
 request_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
 assessment_id BIGINT UNSIGNED NOT NULL,
 PRIMARY KEY (request_id, assessment_id),
 INDEX ix_ai_runtime_assessment (assessment_id, request_id),
 FOREIGN KEY (request_id) REFERENCES ai_bridge_requests(request_id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
