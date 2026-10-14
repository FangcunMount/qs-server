package migration

import (
	"strings"
	"testing"
)

func TestActorColumnRetirementResourcesAndClosedProtectionScope(t *testing.T) {
	up, err := migrations.ReadFile("migrations/mysql/000102_retire_actor_compatibility_columns.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(up)) != "ALTER TABLE `testee`\n  DROP COLUMN `tags`;\n\nALTER TABLE `operators`\n  DROP COLUMN `effective_roles`;" {
		t.Fatal("actor retirement exceeds its two columns")
	}
	down, err := migrations.ReadFile("migrations/mysql/000102_retire_actor_compatibility_columns.down.sql")
	if err != nil || !strings.Contains(string(down), "SIGNAL SQLSTATE '45000'") || strings.Contains(strings.ToUpper(string(down)), "ALTER TABLE") {
		t.Fatal("down cannot recreate empty columns as data recovery")
	}
	r := testCurrentResources(t)
	if r.sql.head != 102 || r.mongo.head != 40 || !r.accepts(101, 40) || !r.accepts(102, 40) || r.accepts(102, 39) || r.accepts(100, 40) {
		t.Fatal("actor retirement requires its exact SQL-only release pair")
	}
}
