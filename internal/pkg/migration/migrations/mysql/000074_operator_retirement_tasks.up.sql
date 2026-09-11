CREATE TABLE operator_retirement_tasks (
 operator_id BIGINT UNSIGNED NOT NULL,
 org_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 stage VARCHAR(16) NOT NULL,
 payload JSON NOT NULL,
 before_state JSON NOT NULL,
 updated_at DATETIME(3) NOT NULL,
 PRIMARY KEY (operator_id),
 KEY idx_operator_retirement_user (user_id),
 KEY idx_operator_retirement_org_stage (org_id, stage)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Durable operator retirement progress and original local facts';
