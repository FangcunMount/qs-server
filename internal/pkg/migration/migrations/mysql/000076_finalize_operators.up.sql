-- Run only after doctor retirement, binding archival and verification in maintenance.
-- Deliberately fail instead of inferring or removing unreviewed bindings.
DROP PROCEDURE IF EXISTS guard_operator_retirement_76;
CREATE PROCEDURE guard_operator_retirement_76()
BEGIN
 IF EXISTS (SELECT 1 FROM clinician c LEFT JOIN staff o ON o.id=c.operator_id
   WHERE c.deleted_at IS NULL AND c.operator_id IS NOT NULL AND (o.id IS NULL OR o.deleted_at IS NULL OR o.is_active<>0)) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Active clinician backend identities remain; complete operator-retire first';
 END IF;
 IF EXISTS (SELECT 1 FROM clinician c LEFT JOIN clinician_operator_binding_archive a ON a.clinician_id=c.id
  WHERE c.operator_id IS NOT NULL AND (a.clinician_id IS NULL OR a.operator_id<>c.operator_id OR a.org_id<>c.org_id OR SHA2(a.before_record,256)<>a.record_sha256)) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Clinician binding archive is missing or invalid';
 END IF;
 IF EXISTS (SELECT 1 FROM clinician_operator_binding_archive a LEFT JOIN operator_retirement_manifests m ON m.migration_id=a.migration_id
  WHERE m.migration_id IS NULL OR m.stage<>'applied_unchanged' OR SHA2(m.payload,256)<>m.payload_sha256) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Retirement manifest is incomplete or invalid';
 END IF;
END;
CALL guard_operator_retirement_76();
DROP PROCEDURE guard_operator_retirement_76;
RENAME TABLE staff TO operators;
ALTER TABLE operators RENAME INDEX uk_staff_org_user TO uk_operators_org_user,
 RENAME INDEX idx_staff_org_deleted_id TO idx_operators_org_deleted_id,
 RENAME INDEX idx_staff_org_active_deleted_id TO idx_operators_org_active_deleted_id;
ALTER TABLE clinician DROP INDEX uk_clinician_org_operator, DROP INDEX idx_operator_id, DROP COLUMN operator_id;
