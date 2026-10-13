//go:build integration && reliable_messaging_m4

package interpretation

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func acceptanceInterpretationAttempt(t *testing.T, scope AcceptanceFailureScope) (interpinput.InterpretationInput, *interpretationrun.InterpretationRun) {
	t.Helper()
	input := interpinput.InterpretationInput{
		OutcomeID: meta.FromUint64(42), Association: report.Association{OrgID: scope.OrgID, TesteeID: scope.TesteeID, AssessmentID: meta.FromUint64(43)},
		Model: report.ModelIdentity{Code: scope.ModelCode},
	}
	run, err := interpretationrun.NewPending(meta.New(), meta.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := run.StartWithLease(now, "probe", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	return input, run
}

func TestInterpretationAcceptanceFailureGateClaimsOnlyOnceInRealMongo(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	now := time.Now()
	scope := AcceptanceFailureScope{Token: meta.New().String(), OrgID: 7, TesteeID: 8, ModelCode: "test-model", StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	claims := db.Collection("interpretation_acceptance_failure_claims")
	raw, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	mongodbtest.RunTestProcess(t, db, "TestInterpretationAcceptanceClaimRestartProcess", "QS_M7_INTERPRETATION_SCOPE="+string(raw), "QS_M7_CLAIM_PHASE=initial")
	for range 2 {
		mongodbtest.RunTestProcess(t, db, "TestInterpretationAcceptanceClaimRestartProcess", "QS_M7_INTERPRETATION_SCOPE="+string(raw))
	}
	if count, err := claims.CountDocuments(t.Context(), bson.M{}); err != nil || count != 1 {
		t.Fatalf("persisted claims=%d err=%v", count, err)
	}
}

func TestInterpretationAcceptanceClaimRestartProcess(t *testing.T) {
	raw := os.Getenv("QS_M7_INTERPRETATION_SCOPE")
	if raw == "" {
		return
	}
	var scope AcceptanceFailureScope
	if err := json.Unmarshal([]byte(raw), &scope); err != nil {
		t.Fatal(err)
	}
	db := mongodbtest.ProcessDatabase(t)
	gate, err := NewAcceptanceFailureGate(db.Collection("interpretation_acceptance_failure_claims"), scope)
	if err != nil {
		t.Fatal(err)
	}
	input, run := acceptanceInterpretationAttempt(t, scope)
	if os.Getenv("QS_M7_CLAIM_PHASE") == "initial" {
		claims := db.Collection("interpretation_acceptance_failure_claims")
		const contenders = 32
		results := make(chan bool, contenders)
		var wg sync.WaitGroup
		for range contenders {
			gate, err := NewAcceptanceFailureGate(claims, scope)
			if err != nil {
				t.Fatal(err)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				claimed, err := gate.TryFail(t.Context(), input, run)
				if err != nil {
					t.Errorf("claim: %v", err)
				}
				results <- claimed
			}()
		}
		wg.Wait()
		close(results)
		won := 0
		for claimed := range results {
			if claimed {
				won++
			}
		}
		if won != 1 {
			t.Fatalf("concurrent claims=%d want1", won)
		}
		return
	}
	if claimed, err := gate.TryFail(t.Context(), input, run); err != nil || claimed {
		t.Fatalf("restarted process claim=%t err=%v", claimed, err)
	}
	if count, err := db.Collection("interpretation_acceptance_failure_claims").CountDocuments(t.Context(), bson.M{}); err != nil || count != 1 {
		t.Fatalf("persisted claims=%d err=%v", count, err)
	}
}
