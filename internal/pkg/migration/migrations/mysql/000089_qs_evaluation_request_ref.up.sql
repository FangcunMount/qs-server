-- Locate the original durable event for an Assessment without scanning
-- Outbox payloads. A retry may create another requested event for the same
-- Assessment, so the business ID is deliberately not unique.
CREATE TABLE qs_rm_evaluation_request_ref (
  event_id VARBINARY(128) NOT NULL PRIMARY KEY,
  assessment_id BIGINT UNSIGNED NOT NULL,
  org_id BIGINT NOT NULL,
  created_at DATETIME(6) NOT NULL DEFAULT (UTC_TIMESTAMP(6)),
  KEY ix_qs_rm_evaluation_request_assessment (assessment_id, event_id)
) ENGINE=InnoDB;
