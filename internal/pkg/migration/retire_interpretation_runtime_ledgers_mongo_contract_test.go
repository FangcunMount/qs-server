package migration

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestRetireInterpretationRuntimeLedgersMongoMigration(t *testing.T) {
	up, err := os.ReadFile("migrations/mongodb/000023_retire_interpretation_runtime_ledgers.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]string
	if err := json.Unmarshal(up, &commands); err != nil {
		t.Fatalf("up migration is not valid JSON: %v", err)
	}
	want := []map[string]string{
		{"drop": "interpretation_admission_failures"},
		{"drop": "interpretation_attention_projections"},
		{"drop": "interpretation_catalog_audit_checkpoints"},
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("retirement commands = %#v, want %#v", commands, want)
	}

	down, err := os.ReadFile("migrations/mongodb/000023_retire_interpretation_runtime_ledgers.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(down) {
		t.Fatal("down migration is not valid JSON")
	}
}
