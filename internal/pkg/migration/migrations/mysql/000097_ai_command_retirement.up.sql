-- Additive phase A. Historical commands are retired only by the reviewed
-- maintenance transaction; no legacy row is migrated or removed here.
ALTER TABLE ai_messaging_operations
  ADD COLUMN retired BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN retirement_evidence JSON NULL,
  ADD COLUMN retired_at DATETIME(6) NULL,
  MODIFY COLUMN body_sha256 CHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL,
  MODIFY COLUMN aggregate_sequence BIGINT UNSIGNED NULL,
  MODIFY COLUMN created_at DATETIME(6) NULL,
  ADD CONSTRAINT chk_ai_messaging_operations_retirement CHECK (
    (retired = FALSE AND body_sha256 IS NOT NULL
      AND aggregate_sequence IS NOT NULL AND aggregate_sequence > 0
      AND created_at IS NOT NULL AND retired_at IS NULL)
    OR
    (retired = TRUE AND body_sha256 IS NULL AND aggregate_sequence IS NULL
      AND created_at IS NULL AND retirement_evidence IS NOT NULL
      AND JSON_TYPE(retirement_evidence) = 'OBJECT'
      AND COALESCE(JSON_TYPE(JSON_EXTRACT(retirement_evidence,'$.version')) = 'INTEGER', FALSE)
      AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(retirement_evidence,'$.version')) = '1', FALSE)
      AND COALESCE(JSON_TYPE(JSON_EXTRACT(retirement_evidence,'$.ownership_verified')) = 'BOOLEAN', FALSE)
      AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(retirement_evidence,'$.ownership_verified')) = 'true', FALSE)
      AND COALESCE(JSON_TYPE(JSON_EXTRACT(retirement_evidence,'$.responsibility_closed')) = 'BOOLEAN', FALSE)
      AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(retirement_evidence,'$.responsibility_closed')) = 'true', FALSE)
      AND COALESCE(JSON_TYPE(JSON_EXTRACT(retirement_evidence,'$.business_terminal')) = 'BOOLEAN', FALSE)
      AND COALESCE(JSON_UNQUOTE(JSON_EXTRACT(retirement_evidence,'$.business_terminal')) = 'true', FALSE)
      AND retired_at IS NOT NULL AND decision = '' AND code = ''
      AND receipt_id IS NULL AND receipt IS NULL AND decided_at IS NULL)
  ),
  ADD INDEX idx_ai_messaging_operations_request_stats
    (organization_id, retired, aggregate_key, kind, decision, command_id),
  ADD INDEX idx_ai_messaging_operations_pending_stats
    (organization_id, retired, kind, decision, aggregate_key, command_id);
