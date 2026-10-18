package main

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRollbackRejectsLegacyTaskEvidenceBeforeReturningAnyRecords(t *testing.T) {
	for _, where := range []string{"before", "after"} {
		t.Run(where, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "repair.jsonl.gz")
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(file)
			encoder := json.NewEncoder(gz)
			current := auditRecord{RunID: "run", Kind: "task", Before: json.RawMessage(`{"id":1,"version":1}`), After: json.RawMessage(`{"id":1,"version":2}`)}
			legacy := current
			if where == "before" {
				legacy.Before = json.RawMessage(`{"id":2,"entry_token":""}`)
			} else {
				legacy.After = json.RawMessage(`{"id":2,"entry_token":null}`)
			}
			for _, record := range []auditRecord{current, legacy} {
				if err := encoder.Encode(record); err != nil {
					t.Fatal(err)
				}
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			records, err := readAuditRecords(path, "run")
			if err == nil || !strings.Contains(err.Error(), "isolated old-schema recovery") || records != nil {
				t.Fatalf("legacy evidence must reject the whole input before rollback: records=%v error=%v", records, err)
			}
		})
	}
}
