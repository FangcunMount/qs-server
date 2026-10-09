package main

import (
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	driversession "go.mongodb.org/mongo-driver/x/mongo/driver/session"
)

// Only the pinned driver's state predicate is exercised offline. This double
// cannot authorize a real source, evidence write, transaction or server response.
type stateOnlyMongoSession struct {
	mongo.Session
	actual *driversession.Client
}

func (s *stateOnlyMongoSession) ClientSession() *driversession.Client { return s.actual }

func TestEvidenceMongoCommitRejectsStartingEvenThoughDriverCallsItRunning(t *testing.T) {
	actual := &driversession.Client{Server: &driversession.Server{}}
	if err := actual.StartTransaction(&driversession.TransactionOptions{ReadConcern: readconcern.Snapshot()}); err != nil {
		t.Fatal(err)
	}
	if !actual.TransactionRunning() || actual.TransactionInProgress() {
		t.Fatal("pinned driver starting semantics changed")
	}
	s := &stateOnlyMongoSession{actual: actual}
	if safeCategory(requireMongoCommitTransaction(s, "required")) != "history_write_mongo_transaction_not_started" {
		t.Fatal("client-only StartTransaction admitted before SQL commit")
	}
	// A real command is required in production. We set this state only to verify
	// the narrow predicate; it does not simulate any actual command/server proof.
	actual.TransactionState = driversession.InProgress
	if requireMongoCommitTransaction(s, "required") != nil {
		t.Fatal("in-progress snapshot rejected")
	}
}
func TestEvidenceMongoCommitStateAndSessionBoundaries(t *testing.T) {
	for _, state := range []driversession.TransactionState{driversession.None, driversession.Starting, driversession.Committed, driversession.Aborted} {
		t.Run(state.String(), func(t *testing.T) {
			s := &stateOnlyMongoSession{actual: &driversession.Client{TransactionState: state, CurrentRc: readconcern.Snapshot()}}
			if requireMongoCommitTransaction(s, "required") == nil {
				t.Fatal("inactive/client-only/ended transaction admitted")
			}
		})
	}
	for _, actual := range []*driversession.Client{nil, {TransactionState: driversession.InProgress}, {TransactionState: driversession.InProgress, CurrentRc: readconcern.Majority()}, {TransactionState: driversession.InProgress, CurrentRc: readconcern.Snapshot(), Terminated: true}} {
		if requireMongoCommitTransaction(&stateOnlyMongoSession{actual: actual}, "required") == nil {
			t.Fatal("wrong native state/read concern/session admitted")
		}
	}
	if requireMongoCommitTransaction(nil, "required") == nil || requireMongoCommitTransaction(&commitCallSession{}, "required") == nil {
		t.Fatal("no native pinned accessor admitted")
	}
	if requireMongoCommitTransaction(nil, "not_required") != nil {
		t.Fatal("AI-only path forced into dummy Mongo command")
	}
	if requireMongoCommitTransaction(nil, "undetermined") == nil {
		t.Fatal("unknown requirement promoted")
	}
}
