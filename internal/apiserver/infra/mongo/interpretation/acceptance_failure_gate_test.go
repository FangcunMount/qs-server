package interpretation

import (
	"context"
	"sync"
	"testing"
	"time"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
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

func TestAcceptanceFailureGateClaimsOneMatchingFirstAttemptAcrossInstances(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	scope := AcceptanceFailureScope{
		Token: "m5-probe-unique-0001", OrgID: 7, TesteeID: 8, ModelCode: "test-model",
		StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
	}
	if err := validateAcceptanceFailureScope(scope, now); err != nil {
		t.Fatal(err)
	}
	store := &acceptanceClaimStore{}
	gate := &AcceptanceFailureGate{claims: store, scope: scope, now: func() time.Time { return now }}
	input := interpinput.InterpretationInput{
		OutcomeID: meta.FromUint64(42), Association: report.Association{OrgID: 7, TesteeID: 8, AssessmentID: meta.FromUint64(43)},
		Model: report.ModelIdentity{Code: "test-model"},
	}
	run, err := interpretationrun.NewPending(meta.New(), meta.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := run.StartWithLease(now, "probe", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	twins := []*AcceptanceFailureGate{gate, {claims: store, scope: scope, now: gate.now}}
	results := make(chan bool, len(twins))
	for _, candidate := range twins {
		go func(candidate *AcceptanceFailureGate) {
			claimed, claimErr := candidate.TryFail(context.Background(), input, run)
			if claimErr != nil {
				t.Errorf("claim: %v", claimErr)
			}
			results <- claimed
		}(candidate)
	}
	claimedCount := 0
	for range twins {
		if <-results {
			claimedCount++
		}
	}
	if claimedCount != 1 || store.count != 2 {
		t.Fatalf("claimed=%d inserts=%d", claimedCount, store.count)
	}
	input.Model.Code = "another-model"
	if claimed, err := gate.TryFail(context.Background(), input, run); err != nil || claimed || store.count != 2 {
		t.Fatalf("different model claimed=%t err=%v inserts=%d", claimed, err, store.count)
	}
	gate.now = func() time.Time { return scope.ExpiresAt }
	input.Model.Code = scope.ModelCode
	if claimed, err := gate.TryFail(context.Background(), input, run); err != nil || claimed || store.count != 2 {
		t.Fatalf("expired probe claimed=%t err=%v inserts=%d", claimed, err, store.count)
	}
}

func TestAcceptanceFailureScopeRejectsBroadOrLongLivedProbe(t *testing.T) {
	now := time.Now()
	valid := AcceptanceFailureScope{
		Token: "m5-probe-unique-0002", OrgID: 7, TesteeID: 8, ModelCode: "test-model",
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
