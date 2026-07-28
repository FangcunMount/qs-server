package migration

import (
	"strings"
	"testing"
)

func TestTaskBusinessCreatedAtMigrationKeepsAuditClockSeparate(t *testing.T) {
	up := readMySQLMigration(t, "000060_add_task_business_created_at.up.sql")
	for _, required := range []string{"`business_created_at` DATETIME(3) NULL", "idx_task_collect_business_created"} {
		if !strings.Contains(up, required) {
			t.Fatalf("task business-time migration missing %q", required)
		}
	}
	if strings.Contains(up, "UPDATE `assessment_task`") || strings.Contains(up, "`created_at` =") {
		t.Fatal("task business-time migration must not rewrite audit created_at")
	}
}
