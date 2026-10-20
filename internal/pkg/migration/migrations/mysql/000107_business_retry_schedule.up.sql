-- Future business retry authorizations. Explicit host installation only.
-- This is not a publish queue: technical attempts and delivery state remain
-- with the SDK, and consumers continue to own business idempotency.
CREATE TABLE qs_retry_schedule_schema (
 id TINYINT NOT NULL PRIMARY KEY,
 version INT NOT NULL
) ENGINE=InnoDB;
CREATE TABLE qs_retry_schedule (
 schedule_key BINARY(32) NOT NULL PRIMARY KEY,
 key_identity BLOB NOT NULL,
 input_hash BINARY(32) NOT NULL,
 plan_hash BINARY(32) NOT NULL,
 owner VARBINARY(128) NOT NULL,
 scope VARBINARY(255) NOT NULL,
 kind VARBINARY(32) NOT NULL,
 subject_id VARBINARY(128) NOT NULL,
 run_id VARBINARY(128) NOT NULL,
 message_id VARBINARY(128) NOT NULL,
 due_index_at DATETIME(6) NOT NULL,
 frozen_plan MEDIUMBLOB NOT NULL,
 state VARBINARY(32) NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1,
 guard_version BIGINT NOT NULL DEFAULT 0,
 dispatch_evidence MEDIUMBLOB NULL,
 UNIQUE KEY qs_retry_message(owner,scope,message_id),
 INDEX qs_retry_due(owner,state,due_index_at,schedule_key),
 INDEX qs_retry_run(owner,scope,kind,run_id,state)
) ENGINE=InnoDB;
INSERT INTO qs_retry_schedule_schema(id,version) VALUES(1,1);
