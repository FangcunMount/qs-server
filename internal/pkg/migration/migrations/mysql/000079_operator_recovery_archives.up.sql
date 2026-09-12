CREATE TABLE operator_recovery_archives (
 request_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 operator_id BIGINT UNSIGNED NOT NULL,
 org_id BIGINT NOT NULL,
 user_id BIGINT NOT NULL,
 retirement_request_id VARCHAR(64) COLLATE utf8mb4_bin NOT NULL,
 recovery_payload LONGTEXT NOT NULL,
 retirement_payload LONGTEXT NOT NULL,
 retirement_before_state LONGTEXT NOT NULL,
 operator_before_state LONGTEXT NOT NULL,
 checksum CHAR(64) NOT NULL,
 created_at DATETIME(3) NOT NULL,
 PRIMARY KEY (request_id),
 UNIQUE KEY uk_operator_recovery_exit (operator_id, retirement_request_id),
 KEY idx_operator_recovery_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Immutable recovery evidence; never restores roles or clinician bindings';
