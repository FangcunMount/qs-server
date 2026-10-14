package migration

import (
	"strings"
	"testing"
)

func TestHistoricalEvidenceRetirementResources(t *testing.T) {
	up, err := migrations.ReadFile("migrations/mysql/000101_retire_historical_event_evidence.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	want := "ALTER TABLE `assessment`\n  DROP CHECK `chk_assessment_historical_lifecycle`,\n  DROP COLUMN `historical_lifecycle_evidence`;\n\nALTER TABLE `evaluation_outcome`\n  DROP CHECK `chk_outcome_historical_committed`,\n  DROP COLUMN `historical_committed_evidence`;"
	if strings.TrimSpace(string(up)) != want {
		t.Fatal("historical evidence retirement exceeds the two reviewed JSON/CHECK objects")
	}
	down, err := migrations.ReadFile("migrations/mysql/000101_retire_historical_event_evidence.down.sql")
	if err != nil || !strings.Contains(string(down), "SIGNAL SQLSTATE '45000'") || strings.Contains(strings.ToUpper(string(down)), "ALTER TABLE") {
		t.Fatal("historical evidence down must refuse rather than recreate empty evidence")
	}
	resources, err := compiledMigrationResources()
	if err != nil || !resources.accepts(101, 39) || resources.mongo.head != 39 {
		t.Fatal("SQL-only historical evidence retirement requires declared 101/39 without Mongo advancement", err)
	}
}
