package modelseed

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type fakePublishedCollection struct {
	counts       []int64
	countFilters []interface{}
	updateFilter interface{}
	update       interface{}
	updateResult *mongo.UpdateResult
	updateErr    error
	updateCalls  int
}

type fakeTransactionRunner struct {
	called     bool
	rolledBack bool
}

func (r *fakeTransactionRunner) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.called = true
	err := fn(ctx)
	if err != nil {
		r.rolledBack = true
	}
	return err
}

func (f *fakePublishedCollection) CountDocuments(_ context.Context, filter interface{}, _ ...*options.CountOptions) (int64, error) {
	f.countFilters = append(f.countFilters, filter)
	if len(f.counts) == 0 {
		return 0, errors.New("unexpected CountDocuments call")
	}
	value := f.counts[0]
	f.counts = f.counts[1:]
	return value, nil
}

func (f *fakePublishedCollection) UpdateMany(_ context.Context, filter, update interface{}, _ ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
	f.updateCalls++
	f.updateFilter = filter
	f.update = update
	if f.updateResult == nil {
		f.updateResult = &mongo.UpdateResult{}
	}
	return f.updateResult, f.updateErr
}

func TestInspectActivePublishedMatchesLegacyKindByStableIdentity(t *testing.T) {
	collection := &fakePublishedCollection{counts: []int64{1, 0, 0}}
	state, err := InspectActivePublished(context.Background(), collection, "gXkk9W", "gXkk9W", "4.0.1")
	if err != nil {
		t.Fatalf("InspectActivePublished() error = %v", err)
	}
	if state.MatchingCount != 1 || state.QuestionnaireOtherModelCount != 0 || state.ModelOtherQuestionnaireCount != 0 {
		t.Fatalf("state = %#v", state)
	}
	matching := collection.countFilters[0].(bson.M)
	if _, exists := matching["model_kind"]; exists {
		t.Fatalf("matching filter must not depend on legacy model_kind: %#v", matching)
	}
	if matching["model_code"] != "gXkk9W" || matching["questionnaire_version"] != "4.0.1" {
		t.Fatalf("matching filter = %#v", matching)
	}
	if err := state.ValidateReplacement(true, true, "gXkk9W", "gXkk9W", "4.0.1"); err != nil {
		t.Fatalf("ValidateReplacement(force) error = %v", err)
	}
}

func TestValidateReplacementRejectsUnrelatedConflicts(t *testing.T) {
	tests := []struct {
		name  string
		state ActivePublishedState
		want  string
	}{
		{name: "same questionnaire other model", state: ActivePublishedState{QuestionnaireOtherModelCount: 1}, want: "other model"},
		{name: "same model other questionnaire", state: ActivePublishedState{ModelOtherQuestionnaireCount: 1}, want: "another questionnaire"},
		{name: "duplicate matching snapshots", state: ActivePublishedState{MatchingCount: 2}, want: "expected at most one"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.state.ValidateReplacement(true, true, "model", "questionnaire", "1.0.0")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateReplacement() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRetireMatchingPublishedChecksMatchedAndModifiedCounts(t *testing.T) {
	collection := &fakePublishedCollection{updateResult: &mongo.UpdateResult{MatchedCount: 0, ModifiedCount: 0}}
	err := RetireMatchingPublished(context.Background(), collection, "gXkk9W", "gXkk9W", "4.0.1", 1, time.Unix(1, 0))
	if err == nil || !strings.Contains(err.Error(), "matched=0 modified=0 want=1") {
		t.Fatalf("RetireMatchingPublished() error = %v", err)
	}
}

func TestRetireMatchingPublishedUsesQuestionnaireIdentityAndNoKind(t *testing.T) {
	now := time.Unix(1, 0)
	collection := &fakePublishedCollection{updateResult: &mongo.UpdateResult{MatchedCount: 1, ModifiedCount: 1}}
	if err := RetireMatchingPublished(context.Background(), collection, "gXkk9W", "gXkk9W", "4.0.1", 1, now); err != nil {
		t.Fatalf("RetireMatchingPublished() error = %v", err)
	}
	filter := collection.updateFilter.(bson.M)
	if _, exists := filter["model_kind"]; exists {
		t.Fatalf("retire filter must not depend on legacy model_kind: %#v", filter)
	}
	want := matchingPublishedFilter("gXkk9W", "gXkk9W", "4.0.1")
	if !reflect.DeepEqual(filter, want) {
		t.Fatalf("retire filter = %#v, want %#v", filter, want)
	}
}

func TestRetireMatchingPublishedDoesNotWriteWhenNothingMatchedAtPreflight(t *testing.T) {
	collection := &fakePublishedCollection{}
	if err := RetireMatchingPublished(context.Background(), collection, "model", "questionnaire", "1.0.0", 0, time.Now()); err != nil {
		t.Fatalf("RetireMatchingPublished() error = %v", err)
	}
	if collection.updateCalls != 0 {
		t.Fatalf("UpdateMany calls = %d, want 0", collection.updateCalls)
	}
}

func TestMongoTransactionRunnerRejectsNilClient(t *testing.T) {
	err := NewMongoTransactionRunner(nil).WithinTransaction(context.Background(), func(context.Context) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "mongo client is nil") {
		t.Fatalf("WithinTransaction() error = %v", err)
	}
}

func TestRunAtomicallyPropagatesPublishedSaveFailureToRollback(t *testing.T) {
	runner := &fakeTransactionRunner{}
	saveErr := errors.New("save published snapshot failed")
	err := RunAtomically(context.Background(), runner, func(context.Context) error {
		return saveErr
	})
	if !errors.Is(err, saveErr) {
		t.Fatalf("RunAtomically() error = %v, want %v", err, saveErr)
	}
	if !runner.called || !runner.rolledBack {
		t.Fatalf("transaction called=%t rolledBack=%t, want true true", runner.called, runner.rolledBack)
	}
}
