package migration

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestReportCatalogAuditMongoMigrationOwnsCheckpointAndBoundedScanIndexes(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000021_add_report_catalog_audit_checkpoint.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatalf("up migration is not valid JSON: %v", err)
	}
	if len(commands) != 3 {
		t.Fatalf("up migration command count = %d, want checkpoint plus two index commands", len(commands))
	}
	for _, token := range []string{
		`"create": "interpretation_catalog_audit_checkpoints"`,
		`"createIndexes": "interpret_report_artifacts"`,
		`"key": { "deleted_at": 1, "assessment_id": 1, "generated_at": -1, "domain_id": -1 }`,
		`"name": "idx_interpret_report_audit_active_assessment_winner"`,
		`"createIndexes": "archived_reports"`,
		`"key": { "deleted_at": 1, "domain_id": 1, "org_id": 1 }`,
		`"name": "idx_archived_report_audit_active_org_domain"`,
	} {
		if !strings.Contains(string(up), token) {
			t.Fatalf("report catalog audit migration does not contain %s", token)
		}
	}

	down, err := os.ReadFile("migrations/mongodb/000021_add_report_catalog_audit_checkpoint.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(down) {
		t.Fatal("down migration is not valid JSON")
	}
	for _, token := range []string{
		`"index": "idx_interpret_report_audit_active_assessment_winner"`,
		`"index": "idx_archived_report_audit_active_org_domain"`,
		`"drop": "interpretation_catalog_audit_checkpoints"`,
	} {
		if !strings.Contains(string(down), token) {
			t.Fatalf("report catalog audit down migration does not contain %s", token)
		}
	}
}
