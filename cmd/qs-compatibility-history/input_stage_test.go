package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHistoryInitialInputsClosedMaterialSet(t *testing.T) {
	for _, kind := range []string{"input_only", "input_and_prepared", "partial_input", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			testSource(t)
			parent := privateTestDir(t)
			j, err := newHistoryWriteJournal(filepath.Join(parent, "captured"), &approvedInputs{request: historyRequest{SourceSHA: sourceSHA, OperationID: "123-1", RunID: "125-1"}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if j.dir.Close() != nil {
					t.Error("journal close")
				}
			}()
			names := historyInitialInputNames()
			selected := names[:]
			if kind == "partial_input" {
				selected = names[:3]
			}
			if kind == "input_and_prepared" {
				selected = append(selected, "prepared-mongo-private.bin", "prepared-sql-private.bin")
			}
			if kind == "foreign" {
				selected = append(selected, "unapproved-input.bin")
			}
			for _, name := range selected {
				f, e := j.create(name)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = f.Write([]byte("owned synthetic input")); e != nil {
					t.Fatal(e)
				}
				if f.Sync() != nil || f.Close() != nil {
					t.Fatal("input close")
				}
			}
			if err = j.record(t.Context(), "initial_inputs_matched", -1, 0, nil); err != nil {
				t.Fatal(err)
			}
			hash, err := j.snapshotMaterials(t.Context())
			if kind == "foreign" || kind == "partial_input" {
				if err == nil || hash != "" {
					t.Fatal("partial or foreign members accepted")
				}
				return
			}
			if err != nil || hash == "" {
				t.Fatal(err)
			}
			raw, e := os.ReadFile(filepath.Join(j.path, "history.materials.private.json"))
			var manifest historyTemporaryMaterialManifest
			if e != nil || json.Unmarshal(raw, &manifest) != nil || len(manifest.Files) != len(selected)+1 {
				t.Fatal("original producer closed list missing")
			}
		})
	}
}

func TestHistoryInitialInputsRejectAbsentHostWithoutAuthority(t *testing.T) {
	if v, err := captureHistoryInitialInputs(t.Context(), nil, nil, nil); err == nil || v != nil {
		t.Fatal("absent host admitted")
	}
}
