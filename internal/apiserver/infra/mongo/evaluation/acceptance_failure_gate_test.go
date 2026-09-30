package evaluation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type acceptanceClaimStore struct {
	mu       sync.Mutex
	inserted bool
	count    int
}

func (s *acceptanceClaimStore) InsertOne(_ context.Context, _ interface{}, _ ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	if s.inserted {
		return nil, mongo.WriteException{WriteErrors: []mongo.WriteError{{Code: 11000}}}
	}
	s.inserted = true
	return &mongo.InsertOneResult{InsertedID: "probe"}, nil
}

func TestAcceptanceFailureGateClaimsOnlyOneMatchingPreModelAttempt(t *testing.T) {
	now := time.Now()
	scope := AcceptanceFailureScope{
		Token: "m5-eval-probe-unique-0001", OrgID: 7, TesteeID: 8, ModelCode: "test-model",
		StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
	}
	store := &acceptanceClaimStore{}
	gate := &AcceptanceFailureGate{claims: store, scope: scope, now: func() time.Time { return now }}
	a, err := assessment.NewAssessment(7, testee.NewID(8),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "v1"),
		assessment.NewAnswerSheetRef(meta.FromUint64(42)), assessment.NewAdhocOrigin(),
		assessment.WithID(meta.FromUint64(43)))
	if err != nil {
		t.Fatal(err)
	}
	input := &evaluationinput.InputSnapshot{Model: &evaluationinput.ModelSnapshot{Code: "test-model"}}
	run := evalrun.NewEvaluationRun(43)
	if err := run.Start(now); err != nil {
		t.Fatal(err)
	}
	if claimed, err := gate.TryFail(t.Context(), a, input, &run); err != nil || claimed || store.count != 0 {
		t.Fatalf("unfrozen run claimed=%t err=%v inserts=%d", claimed, err, store.count)
	}
	if err := run.AttachInputSnapshot("frozen-input-ref"); err != nil {
		t.Fatal(err)
	}
	twins := []*AcceptanceFailureGate{gate, {claims: store, scope: scope, now: gate.now}}
	results := make(chan bool, len(twins))
	for _, candidate := range twins {
		go func(candidate *AcceptanceFailureGate) {
			claimed, claimErr := candidate.TryFail(context.Background(), a, input, &run)
			if claimErr != nil {
				t.Errorf("claim: %v", claimErr)
			}
			results <- claimed
		}(candidate)
	}
	count := 0
	for range twins {
		if <-results {
			count++
		}
	}
	if count != 1 || store.count != 2 {
		t.Fatalf("claimed=%d inserts=%d, want 1/2", count, store.count)
	}
	input.Model.Code = "other-model"
	if claimed, err := gate.TryFail(t.Context(), a, input, &run); err != nil || claimed || store.count != 2 {
		t.Fatalf("different model claimed=%t err=%v inserts=%d", claimed, err, store.count)
	}
	input.Model.Code = scope.ModelCode
	if err := run.Fail(now, evalrun.Failure{Kind: evalrun.FailureKindDependency, Retryable: true}); err != nil {
		t.Fatal(err)
	}
	retry := evalrun.NextEvaluationRun(run)
	if err := retry.Start(now); err != nil {
		t.Fatal(err)
	}
	if claimed, err := gate.TryFail(t.Context(), a, input, &retry); err != nil || claimed || store.count != 2 {
		t.Fatalf("retry claimed=%t err=%v inserts=%d", claimed, err, store.count)
	}
	gate.now = func() time.Time { return scope.ExpiresAt }
	if claimed, err := gate.TryFail(t.Context(), a, input, &run); err != nil || claimed || store.count != 2 {
		t.Fatalf("expired gate claimed=%t err=%v inserts=%d", claimed, err, store.count)
	}
}

func TestAcceptanceFailureScopeRejectsBroadOrLongLivedProbe(t *testing.T) {
	now := time.Now()
	valid := AcceptanceFailureScope{
		Token: "m5-eval-probe-unique-0002", OrgID: 7, TesteeID: 8, ModelCode: "test-model",
		StartsAt: now, ExpiresAt: now.Add(5 * time.Minute),
	}
	for _, mutate := range []func(*AcceptanceFailureScope){
		func(s *AcceptanceFailureScope) { s.OrgID = 0 },
		func(s *AcceptanceFailureScope) { s.TesteeID = 0 },
		func(s *AcceptanceFailureScope) { s.ModelCode = "" },
		func(s *AcceptanceFailureScope) { s.Token = "short" },
		func(s *AcceptanceFailureScope) { s.ExpiresAt = now.Add(11 * time.Minute) },
	} {
		scope := valid
		mutate(&scope)
		if err := validateAcceptanceFailureScope(scope, now); err == nil {
			t.Fatalf("invalid scope accepted: %+v", scope)
		}
	}
	if err := validateAcceptanceFailureScope(valid, now); err != nil {
		t.Fatal(err)
	}
}
