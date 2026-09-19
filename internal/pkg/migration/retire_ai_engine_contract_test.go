package migration

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestAIEngineRetirementMigrationHasExactWhitelistAndNoFakeRestore(t *testing.T) {
	raw, err := os.ReadFile("migrations/mongodb/000035_retire_ai_engine.up.json")
	if err != nil {
		t.Fatal(err)
	}
	var commands []map[string]string
	if err := json.Unmarshal(raw, &commands); err != nil {
		t.Fatal(err)
	}
	want := make([]map[string]string, len(aiEngineRetirementCollections))
	for i, name := range aiEngineRetirementCollections {
		want[i] = map[string]string{"drop": name}
	}
	if !reflect.DeepEqual(commands, want) || len(commands) != 9 {
		t.Fatalf("unexpected retirement commands: %v", commands)
	}
	raw, err = os.ReadFile("migrations/mongodb/000035_retire_ai_engine.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &commands); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 0 {
		t.Fatal("inverse migration must not pretend to restore retired data")
	}
}
