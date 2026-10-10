package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryWriteJournalActualPrivateFilesNeverBecomeAuthority(t *testing.T) {
	parent := t.TempDir()
	if os.Chmod(parent, 0o700) != nil {
		t.Fatal("mode")
	}
	dir := filepath.Join(parent, "new-operation")
	input := &approvedInputs{request: historyRequest{SourceSHA: strings.Repeat("a", 40), OperationID: "100-1", RunID: "101-1"}}
	journal, e := newHistoryWriteJournal(dir, input)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if journal.dir.Close() != nil {
			t.Error("close")
		}
	}()
	if e = journal.record(context.Background(), "sql_commit_intent", -1, 0, nil); e != nil {
		t.Fatal(e)
	}
	if e = journal.record(context.Background(), "sql_commit_unknown", -1, 0, nil); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(dir, "journal-2.json"))
	if e != nil {
		t.Fatal(e)
	}
	var record map[string]any
	if json.Unmarshal(raw, &record) != nil {
		t.Fatal("json")
	}
	for _, key := range []string{"complete", "execution_ready", "cas_authorized", "drop_ready"} {
		if record[key] != false {
			t.Fatal("journal promoted authority", key)
		}
	}
	if record["stage"] != "sql_commit_unknown" {
		t.Fatal("unknown response hidden")
	}
	if _, e = newHistoryWriteJournal(dir, input); e == nil {
		t.Fatal("partial/existing directory adopted")
	}
	if e = journal.record(context.Background(), "drop_authorized", -1, 0, nil); e == nil {
		t.Fatal("unsupported stage accepted")
	}
}
func TestHistoryWriteJournalDoesNotFollowRenamedOutputDirectory(t *testing.T) {
	parent := t.TempDir()
	if os.Chmod(parent, 0o700) != nil {
		t.Fatal("mode")
	}
	dir := filepath.Join(parent, "out")
	input := &approvedInputs{request: historyRequest{SourceSHA: strings.Repeat("a", 40), OperationID: "100-1", RunID: "101-1"}}
	journal, e := newHistoryWriteJournal(dir, input)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if journal.dir.Close() != nil {
			t.Error("close")
		}
	}()
	if os.Rename(dir, dir+"-moved") != nil {
		t.Fatal("rename")
	}
	if os.Mkdir(dir, 0o700) != nil {
		t.Fatal("replace")
	}
	if journal.record(context.Background(), "prepared", 0, 1, nil) == nil {
		t.Fatal("changed output inode/path accepted")
	}
}

func TestHistoryWriteCallerRejectsUnqualifiedInputs(t *testing.T) {
	report, e := executeHistoricalEvidenceWrite(context.Background(), nil, nil, filepath.Join(t.TempDir(), "must-not-create"))
	if e == nil {
		t.Fatal("missing actual host and approved inputs admitted")
	}
	if report.LimitedEventPersistenceObserved || report.CASComplete || report.DropReady || report.MutationBackendEnabled || report.ActualSQLCommitResponse || report.ActualMongoCommitResponse {
		t.Fatal("input rejection promoted actual persistence or authority")
	}
}

func TestHistoryWriteMaterialManifestFromOriginalClosedSpoolsAndJournals(t *testing.T) {
	prior := sourceSHA
	sourceSHA = strings.Repeat("b", 40)
	defer func() { sourceSHA = prior }()
	parent, _ := filepath.EvalSymlinks(t.TempDir())
	_ = os.Chmod(parent, 0700)
	j, err := newHistoryWriteJournal(filepath.Join(parent, "write"), &approvedInputs{request: historyRequest{SourceSHA: strings.Repeat("a", 40), OperationID: "100-1", RunID: "101-1"}})
	if err != nil {
		t.Fatal(err)
	}
	defer func(close func() error) { _ = close() }(j.dir.Close)
	for _, name := range []string{"prepared-mongo-private.bin", "prepared-sql-private.bin"} {
		f, e := j.create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write([]byte("private-body")); e != nil {
			t.Fatal(e)
		}
		if f.Sync() != nil || f.Close() != nil {
			t.Fatal("close")
		}
	}
	for _, stage := range []string{"sql_commit_success", "limited_event_readback_finished"} {
		if j.record(context.Background(), stage, 0, 1, nil) != nil {
			t.Fatal("journal")
		}
	}
	hash, err := j.snapshotMaterials(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(j.path, "history.materials.private.json"))
	var m historyTemporaryMaterialManifest
	if strictDecode(raw, &m) != nil || rawHash(raw) != hash || m.Version != 1 || m.MaxSpoolBytes != 16<<30 || m.JournalSequence != 2 || len(m.Files) != 4 {
		t.Fatal("actual member manifest")
	}
	for _, v := range m.Files {
		body, _ := os.ReadFile(filepath.Join(j.path, v.Name))
		info, _ := os.Stat(filepath.Join(j.path, v.Name))
		if v.Bytes != int64(len(body)) || v.SHA256 != rawHash(body) || v.UID != uint32(os.Getuid()) || v.Inode == 0 || v.Mode != 0600 || !info.Mode().IsRegular() {
			t.Fatal("actual source metadata")
		}
	}
	if _, err = j.snapshotMaterials(context.Background()); err == nil {
		t.Fatal("producer snapshot reissued")
	}
	if strings.Contains(string(raw), "private-body") || strings.Contains(string(raw), "complete") || strings.Contains(string(raw), "permission") {
		t.Fatal("descriptor exposed body or authority")
	}
}

func TestHistoryWriteMaterialManifestUsesSevenInputProtocolOnly(t *testing.T) {
	for _, kind := range []string{"seven", "six", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			testSource(t)
			j, err := newHistoryWriteJournal(filepath.Join(privateTestDir(t), "write"), &approvedInputs{request: historyRequest{SourceSHA: sourceSHA, OperationID: "100-1", RunID: "101-1"}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if j.dir.Close() != nil {
					t.Error("journal close")
				}
			}()
			fixed := historyInitialInputNames()
			names := append([]string(nil), fixed[:]...)
			if kind != "six" {
				names = append(names, historyInputOwnerSQLName)
			}
			if kind == "mixed" {
				names = append(names, "prepared-mongo-private.bin", "prepared-sql-private.bin")
			}
			for _, name := range names {
				f, e := j.create(name)
				if e != nil {
					t.Fatal(e)
				}
				if f.Sync() != nil || f.Close() != nil {
					t.Fatal("original input close")
				}
			}
			if j.record(t.Context(), "initial_inputs_matched", -1, 0, nil) != nil {
				t.Fatal("original journal")
			}
			hash, err := j.snapshotMaterials(t.Context())
			if kind != "seven" {
				if err == nil || hash != "" {
					t.Fatal("partial or mixed producer protocol admitted")
				}
				return
			}
			raw, err := os.ReadFile(filepath.Join(j.path, "history.materials.private.json"))
			var manifest historyTemporaryMaterialManifest
			if err != nil || hash != rawHash(raw) || strictDecode(raw, &manifest) != nil || manifest.Version != 2 || len(manifest.Files) != 8 || manifest.Files[6].Name != historyInputOwnerSQLName || manifest.JournalSequence != 1 {
				t.Fatal("actual seven-input manifest not bound")
			}
		})
	}
}

func TestHistoryWriteMaterialManifestRejectsUnregisteredOrReplacedObjects(t *testing.T) {
	for _, mutation := range []string{"extra", "replace", "hardlink", "missing", "unknown", "oversize"} {
		t.Run(mutation, func(t *testing.T) {
			parent, _ := filepath.EvalSymlinks(t.TempDir())
			_ = os.Chmod(parent, 0700)
			j, e := newHistoryWriteJournal(filepath.Join(parent, "write"), &approvedInputs{request: historyRequest{SourceSHA: strings.Repeat("a", 40), OperationID: "100-1", RunID: "101-1"}})
			if e != nil {
				t.Fatal(e)
			}
			defer func(close func() error) { _ = close() }(j.dir.Close)
			for _, name := range []string{"prepared-mongo-private.bin", "prepared-sql-private.bin"} {
				f, e := j.create(name)
				if e != nil || f.Close() != nil {
					t.Fatal("spool")
				}
			}
			p := filepath.Join(j.path, "prepared-mongo-private.bin")
			switch mutation {
			case "extra":
				_ = os.WriteFile(filepath.Join(j.path, "unknown"), nil, 0600)
			case "replace":
				_ = os.Rename(p, p+"-old")
				_ = os.WriteFile(p, nil, 0600)
				_ = os.Remove(p + "-old")
			case "hardlink":
				_ = os.Link(p, filepath.Join(parent, "linked"))
			case "missing":
				_ = os.Remove(p)
			case "unknown":
				j.unknown = true
			case "oversize":
				if os.Truncate(p, (16<<30)+1) != nil {
					t.Fatal("sparse bound")
				}
			}
			if _, e = j.snapshotMaterials(context.Background()); e == nil {
				t.Fatal("unbound source accepted")
			}
			if _, e = os.Lstat(filepath.Join(j.path, "history.materials.private.json")); !os.IsNotExist(e) {
				t.Fatal("failed source published descriptor")
			}
		})
	}
}

func TestBoundedHistoryWriterDoesNotOpenScopeFromEmptyPrivateInputs(t *testing.T) {
	var host historyDatabase
	if _, report, err := host.writeHistoricalAI(t.Context(), nil, nil); err == nil || report.CommitState != "not_attempted" || report.ActualSQLCommitResponse {
		t.Fatal("empty AI private capability opened write scope")
	}
	sql, mongo, refs, report, err := host.writeHistoricalComponent(t.Context(), nil, nil, nil, 0, nil, nil, nil)
	if err == nil || len(sql) != 0 || mongo != nil || refs != 0 || report.ActualSQLCommitResponse || report.ActualMongoCommitResponse || report.DropReady || report.CASComplete {
		t.Fatal("empty fresh component promoted effect or commit")
	}
}
