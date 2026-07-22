package migration

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestStatisticsMongoCollectorIndexesMatchSourceScans(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000018_add_statistics_collector_indexes.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]any
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatalf("up migration is not valid JSON: %v", err)
	}
	if len(commands) != 3 {
		t.Fatalf("create index command count=%d want 3", len(commands))
	}
	text := string(up)
	for _, token := range []string{
		`"idx_answersheets_statistics_org_filled"`,
		`"key": { "org_id": 1, "filled_at": 1, "domain_id": 1 }`,
		`"partialFilterExpression": { "deleted_at": null }`,
		`"idx_artifacts_statistics_org_generated"`,
		`"key": { "org_id": 1, "generated_at": 1, "domain_id": 1 }`,
		`"idx_interpretation_runs_statistics_org_failed"`,
		`"key": { "org_id": 1, "status": 1, "finished_at": 1, "domain_id": 1 }`,
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("mongo collector index migration does not contain %s", token)
		}
	}

	down, err := os.ReadFile("migrations/mongodb/000018_add_statistics_collector_indexes.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(down) {
		t.Fatal("down migration is not valid JSON")
	}
	for _, index := range []string{"idx_answersheets_statistics_org_filled", "idx_artifacts_statistics_org_generated", "idx_interpretation_runs_statistics_org_failed"} {
		if !strings.Contains(string(down), index) {
			t.Fatalf("mongo collector index down migration does not remove %s", index)
		}
	}
}
