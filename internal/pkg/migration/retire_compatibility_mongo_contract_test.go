package migration

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestRetireEmptyCompatibilityCollectionsMongoMigration(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000022_retire_empty_compatibility_collections.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]string
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatalf("up migration is not valid JSON: %v", err)
	}
	want := []map[string]string{
		{"drop": "answersheet_submit_idempotency"},
		{"drop": "archived_reports"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("retirement commands = %#v, want %#v", commands, want)
	}

	down, err := os.ReadFile("migrations/mongodb/000022_retire_empty_compatibility_collections.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(down) {
		t.Fatal("down migration is not valid JSON")
	}
}
