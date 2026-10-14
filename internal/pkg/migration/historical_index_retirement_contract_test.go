package migration

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHistoricalIndexRetirementResources(t *testing.T) {
	up, err := migrations.ReadFile("migrations/mongodb/000040_retire_historical_event_indexes.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]string
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatal(err)
	}
	want := []map[string]string{
		{"dropIndexes": "answersheets", "index": "uk_answersheet_legacy_submission_event_id"},
		{"dropIndexes": "answersheets", "index": "idx_answersheet_legacy_submission_audit"},
		{"dropIndexes": "report_generations", "index": "uk_generation_historical_generated_event_id"},
		{"dropIndexes": "report_generations", "index": "idx_generation_historical_generated_audit"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatal("Mongo40 must only drop the four reviewed historical indexes")
	}
	commands = nil
	down, err := migrations.ReadFile("migrations/mongodb/000040_retire_historical_event_indexes.down.json")
	if err != nil || json.Unmarshal(down, &commands) != nil || !reflect.DeepEqual(commands, []map[string]string{{"historical_index_retirement_down_unsupported": "restore a verified full backup pair or use approved forward repair"}}) {
		t.Fatal("historical index down must explicitly refuse automatic recreation", err)
	}
	r := testCurrentResources(t)
	if !r.sql.contains(101) || !r.mongo.contains(40) || !r.accepts(101, 39) || !r.accepts(101, 40) || r.accepts(100, 40) {
		t.Fatal("Mongo-only retirement requires101/40 and the safe101/39 start")
	}
}
