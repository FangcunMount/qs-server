package migration

import (
	"strings"
	"testing"
)

func TestLegacyMaintenanceRetirementResourcesAndClosedProtectionScope(t *testing.T) {
	up, err := migrations.ReadFile("migrations/mysql/000103_retire_legacy_maintenance_tables.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	want := "DROP TABLE `testee_store_migration_reversions`;\nDROP TABLE `testee_store_migration_items`;\nDROP TABLE `testee_store_migration_manifests`;\nDROP TABLE `clinician_operator_binding_archive`;\nDROP TABLE `operator_retirement_manifests`;"
	if strings.TrimSpace(string(up)) != want {
		t.Fatal("maintenance retirement exceeds its five reviewed tables")
	}
	down, err := migrations.ReadFile("migrations/mysql/000103_retire_legacy_maintenance_tables.down.sql")
	if err != nil || !strings.Contains(string(down), "SIGNAL SQLSTATE '45000'") || strings.Contains(strings.ToUpper(string(down)), "CREATE TABLE") {
		t.Fatal("down cannot recreate empty tables as historical data recovery")
	}
	r := testCurrentResources(t)
	if r.sql.head != 103 || r.mongo.head != 40 || !r.accepts(102, 40) || !r.accepts(103, 40) || r.accepts(103, 39) || r.accepts(102, 41) {
		t.Fatal("maintenance retirement requires its exact SQL-only release pair")
	}
}
