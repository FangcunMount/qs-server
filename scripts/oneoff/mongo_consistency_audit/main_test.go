package main

import (
	"reflect"
	"testing"

	mongoconsistency "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
)

func TestParseScope(t *testing.T) {
	t.Parallel()

	got, err := parseScope(" generation_run, retry_outbox,generation_run ")
	if err != nil {
		t.Fatalf("parse scope: %v", err)
	}
	want := []mongoconsistency.Phase{
		mongoconsistency.PhaseGenerationRun,
		mongoconsistency.PhaseRetryOutbox,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
}

func TestParseScopeRejectsUnknownValue(t *testing.T) {
	t.Parallel()

	if _, err := parseScope("repair"); err == nil {
		t.Fatal("expected unknown scope to be rejected")
	}
}
