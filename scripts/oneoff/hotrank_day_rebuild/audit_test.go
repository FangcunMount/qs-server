package main

import (
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOperationAuditRejectsConflictingReuseAndPreservesOriginalIntent(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	intent := operationIntent{Mode: "apply", Day: "20261003", RequestID: uuid.NewString(), Operator: "operator-a", Fingerprint: strings.Repeat("ab", 32), TargetFingerprint: strings.Repeat("cd", 32)}
	j, err := openOperationJournal(dir, intent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openOperationJournal(dir, intent); err != nil {
		t.Fatal("same original intent", err)
	}
	changed := intent
	changed.Operator = "operator-b"
	if _, err := openOperationJournal(dir, changed); err == nil {
		t.Fatal("operator reuse accepted")
	}
	changed = intent
	changed.TargetFingerprint = strings.Repeat("ef", 32)
	if _, err := openOperationJournal(dir, changed); err == nil {
		t.Fatal("cross-target request accepted")
	}
	if err := j.confirm(intent); err != nil {
		t.Fatal(err)
	}
	if err := j.confirm(changed); err == nil {
		t.Fatal("changed confirmation accepted")
	}
	for _, kind := range []string{"intent", "confirmed"} {
		stat, err := os.Stat(filepath.Join(dir, intent.RequestID+"."+kind+".json"))
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatal("protected audit", err)
		}
	}
}
func TestOperationAuditRefusesPublicDirectoryAndPartialOriginalRecord(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	intent := operationIntent{Mode: "apply", Day: "20261003", RequestID: uuid.NewString(), Operator: "operator", Fingerprint: strings.Repeat("ab", 32)}
	if _, err := openOperationJournal(dir, intent); err == nil {
		t.Fatal("public directory accepted")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, intent.RequestID+".intent.json"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := openOperationJournal(dir, intent); err == nil {
		t.Fatal("partial intent overwritten")
	}
}
