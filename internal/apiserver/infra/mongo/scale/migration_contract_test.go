package scale

import (
	"os"
	"strings"
	"testing"
)

func TestScaleVersionMigrationBackfillsAndIndexesScaleVersion(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../pkg/migration/migrations/mongodb/000005_add_scale_version.up.json")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(data)
	for _, token := range []string{
		"scale_version",
		"questionnaire_version",
		"1.0.0",
		"idx_scales_code_version_deleted",
	} {
		if !strings.Contains(sql, token) {
			t.Fatalf("migration missing %q", token)
		}
	}
}
