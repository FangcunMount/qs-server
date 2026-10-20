package migration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRetryScheduleResourcesRequireCompletePairAndRetainAuthorizations(t *testing.T) {
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	if !r.accepts(107, 42) || !r.accepts(106, 41) || r.accepts(107, 41) || r.accepts(106, 42) {
		t.Fatal("retry schema must use an explicit full pair")
	}
	down, err := migrations.ReadFile("migrations/mysql/000107_business_retry_schedule.down.sql")
	if err != nil || !strings.Contains(string(down), "SIGNAL") || strings.Contains(strings.ToUpper(string(down)), "DROP TABLE") {
		t.Fatal("downgrade must preserve business authorizations", err)
	}
	raw, err := migrations.ReadFile("migrations/mongodb/000042_business_retry_schedule.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]json.RawMessage
	if err = json.Unmarshal(raw, &commands); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 4 || string(commands[3]["insert"]) != `"qs_retry_schedule_schema"` || strings.Contains(string(raw), "expireAfterSeconds") {
		t.Fatal("schema marker must follow explicit installation; no automatic evidence deletion")
	}
}
