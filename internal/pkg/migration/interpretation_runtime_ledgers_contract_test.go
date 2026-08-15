package migration

import (
	"strings"
	"testing"
)

func TestInterpretationRuntimeLedgerMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000068_migrate_interpretation_runtime_ledgers.up.sql")
	down := readMySQLMigration(t, "000068_migrate_interpretation_runtime_ledgers.down.sql")

	for _, table := range []string{
		"interpretation_admission_failure",
		"interpretation_catalog_audit_checkpoint",
		"interpretation_attention_projection",
	} {
		if !strings.Contains(up, "CREATE TABLE `"+table+"`") {
			t.Fatalf("up migration does not create %s", table)
		}
		if !strings.Contains(down, "DROP TABLE IF EXISTS `"+table+"`") {
			t.Fatalf("down migration does not drop %s", table)
		}
	}

	for _, constraint := range []string{
		"uk_interpretation_admission_failure_fingerprint",
		"idx_interpretation_admission_failure_operations",
		"PRIMARY KEY (`checkpoint_key`)",
		"idx_interpretation_attention_projection_status_updated",
	} {
		if !strings.Contains(up, constraint) {
			t.Fatalf("up migration is missing %s", constraint)
		}
	}
}
