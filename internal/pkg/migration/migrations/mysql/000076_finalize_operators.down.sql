-- This restores structure and archived bindings only; never reactivate operators or IAM grants.
DROP PROCEDURE IF EXISTS guard_operator_restore_76;
CREATE PROCEDURE guard_operator_restore_76()
BEGIN
 IF EXISTS (SELECT 1 FROM operator_retirement_manifests m WHERE m.stage='applied_unchanged' AND
  (JSON_EXTRACT(m.payload,'$.binding_count') IS NULL OR
   CAST(JSON_UNQUOTE(JSON_EXTRACT(m.payload,'$.binding_count')) AS UNSIGNED) <>
   (SELECT COUNT(*) FROM clinician_operator_binding_archive a WHERE a.migration_id=m.migration_id))) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Cannot restore incomplete retirement archive';
 END IF;
 IF EXISTS (SELECT 1 FROM clinician_operator_binding_archive a LEFT JOIN operator_retirement_manifests m ON m.migration_id=a.migration_id
  WHERE m.migration_id IS NULL OR SHA2(a.before_record,256)<>a.record_sha256 OR SHA2(m.payload,256)<>m.payload_sha256) THEN
  SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='Cannot restore corrupt retirement archive';
 END IF;
END;
CALL guard_operator_restore_76();
DROP PROCEDURE guard_operator_restore_76;
ALTER TABLE clinician ADD COLUMN operator_id BIGINT UNSIGNED NULL,
 ADD UNIQUE KEY uk_clinician_org_operator(org_id,operator_id), ADD KEY idx_operator_id(operator_id);
UPDATE clinician c JOIN clinician_operator_binding_archive a ON a.clinician_id=c.id AND a.org_id=c.org_id
 SET c.operator_id=a.operator_id WHERE SHA2(a.before_record,256)=a.record_sha256;
ALTER TABLE operators RENAME INDEX uk_operators_org_user TO uk_staff_org_user,
 RENAME INDEX idx_operators_org_deleted_id TO idx_staff_org_deleted_id,
 RENAME INDEX idx_operators_org_active_deleted_id TO idx_staff_org_active_deleted_id;
RENAME TABLE operators TO staff;
