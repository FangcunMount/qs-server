package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
)

// This unit double only observes whether the host calls CommitTransaction. It
// is never passed into the real source/qualification/evidence-write pipeline.
type commitCallSession struct {
	mongo.Session
	calls    int
	response error
}

func (s *commitCallSession) CommitTransaction(context.Context) error { s.calls++; return s.response }
func commitUnitJournal(t *testing.T) *historyWriteJournal {
	t.Helper()
	parent := t.TempDir()
	if os.Chmod(parent, 0700) != nil {
		t.Fatal("parent")
	}
	a := &approvedInputs{request: historyRequest{SourceSHA: strings.Repeat("a", 40), OperationID: "100-1", RunID: "101-1"}}
	j, err := newHistoryWriteJournal(filepath.Join(parent, "write"), a)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if j.dir.Close() != nil {
			t.Error("close")
		}
	})
	return j
}
func TestEvidenceWriteMongoNotRequiredNeverCallsClientCommit(t *testing.T) {
	j := commitUnitJournal(t)
	s := &commitCallSession{response: errors.New("must not be reached")}
	p := preparedWriteDiagnostic{ActualSQLCommitResponse: true, MongoCommitRequirement: "not_required"}
	if err := commitPreparedMongo(t.Context(), s, j, &p, -1); err != nil {
		t.Fatal(err)
	}
	if s.calls != 0 || p.ActualMongoCommitResponse || p.CommitState != "sql_committed_mongo_not_required" {
		t.Fatal("empty Mongo transaction promoted to actual response")
	}
	raw, err := os.ReadFile(filepath.Join(j.path, "journal-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"stage":"mongo_commit_not_required"`) || !strings.Contains(string(raw), `"drop_ready":false`) {
		t.Fatal("skip not durably distinguished")
	}
}
func TestEvidenceWriteMongoRequiredKeepsUnknownAndJournalFailure(t *testing.T) {
	for _, kind := range []string{"unknown", "journal_failed", "success"} {
		t.Run(kind, func(t *testing.T) {
			j := commitUnitJournal(t)
			s := &commitCallSession{}
			p := preparedWriteDiagnostic{ActualSQLCommitResponse: true, MongoCommitRequirement: "required", PreparedPages: 1, EventReferences: 1}
			if kind == "unknown" {
				s.response = errors.New("unknown")
			}
			if kind == "journal_failed" {
				j.unknown = true
			}
			err := commitPreparedMongo(t.Context(), s, j, &p, -1)
			if s.calls != 1 || !p.ActualSQLCommitResponse {
				t.Fatal("actual SQL response hidden or duplicate Mongo commit")
			}
			if kind == "unknown" {
				if safeCategory(err) != "history_write_commit_unknown" || p.ActualMongoCommitResponse || p.CommitState != "sql_committed_mongo_unknown" {
					t.Fatal("unknown promoted")
				}
			}
			if kind == "journal_failed" {
				if err == nil || !p.ActualMongoCommitResponse || p.CommitState != "both_responses_success_non_atomic" {
					t.Fatal("returned response hidden or journal failure ignored")
				}
			}
			if kind == "success" {
				if err != nil || !p.ActualMongoCommitResponse {
					t.Fatal("response not retained", err)
				}
			}
		})
	}
}
func TestEvidenceWriteNotRequiredRejectsEventWorkAndUnknownJournal(t *testing.T) {
	for _, kind := range []string{"events", "pages", "prior_response", "journal_unknown"} {
		t.Run(kind, func(t *testing.T) {
			j := commitUnitJournal(t)
			s := &commitCallSession{}
			p := preparedWriteDiagnostic{ActualSQLCommitResponse: true, MongoCommitRequirement: "not_required"}
			switch kind {
			case "events":
				p.EventReferences = 1
			case "pages":
				p.PreparedPages = 1
			case "prior_response":
				p.ActualMongoCommitResponse = true
			case "journal_unknown":
				j.unknown = true
			}
			if commitPreparedMongo(t.Context(), s, j, &p, -1) == nil || s.calls != 0 {
				t.Fatal("contradictory/no-journal skip admitted")
			}
		})
	}
}
func TestEvidenceWriteCompletionRequiresTrueCommitAndCompleteReadback(t *testing.T) {
	ai := evidenceWriteReport{MongoCommitRequirement: "not_required", CommitState: "sql_committed_mongo_not_required", ActualSQLCommitResponse: true, AIOriginalCommands: 3, AISourceReferences: 4, AICommandPersistenceComplete: true}
	events := evidenceWriteReport{MongoCommitRequirement: "required", CommitState: "both_responses_success_non_atomic", ActualSQLCommitResponse: true, ActualMongoCommitResponse: true, EventReferences: 2, PreparedPages: 1, ReadBackPages: 1, EventPersistenceObserved: true}
	if !evidenceWriteCompleted(ai, nil) || !evidenceWriteCompleted(events, nil) {
		t.Fatal("real scoped success rejected")
	}
	for _, kind := range []string{"fake_mongo", "wrong_state", "missing_ai_readback", "independent_readback_failed", "unknown_requirement", "page_missing"} {
		t.Run(kind, func(t *testing.T) {
			r := ai
			var err error
			switch kind {
			case "fake_mongo":
				r.ActualMongoCommitResponse = true
			case "wrong_state":
				r.CommitState = "both_responses_success_non_atomic"
			case "missing_ai_readback":
				r.AICommandPersistenceComplete = false
			case "independent_readback_failed":
				err = fixedError("history_write_independent_readback_failed")
			case "unknown_requirement":
				r.MongoCommitRequirement = "undetermined"
			case "page_missing":
				r = events
				r.ReadBackPages = 0
			}
			if evidenceWriteCompleted(r, err) {
				t.Fatal("incomplete/contradictory commit/readback promoted")
			}
		})
	}
}
