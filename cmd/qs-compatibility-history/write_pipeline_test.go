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
