//go:build integration && reliable_messaging_m4

package evaluation

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func TestEvaluationAcceptanceFailureGateClaimsOnlyOnceInRealMongo(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	now := time.Now()
	scope := AcceptanceFailureScope{
		Token: meta.New().String(), OrgID: 7, TesteeID: 8, ModelCode: "test-model",
		StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute),
	}
	claims := db.Collection("evaluation_acceptance_failure_claims")
	raw, err := json.Marshal(scope)
	if err != nil {
		t.Fatal(err)
	}
	mongodbtest.RunTestProcess(t, db, "TestEvaluationAcceptanceClaimRestartProcess", "QS_M7_EVALUATION_SCOPE="+string(raw), "QS_M7_CLAIM_PHASE=initial")
	for range 2 {
		mongodbtest.RunTestProcess(t, db, "TestEvaluationAcceptanceClaimRestartProcess", "QS_M7_EVALUATION_SCOPE="+string(raw))
	}

	if count, err := claims.CountDocuments(t.Context(), bson.M{}); err != nil || count != 1 {
		t.Fatalf("persisted claims=%d err=%v", count, err)
	}
}

func acceptanceEvaluationAttempt(t *testing.T, scope AcceptanceFailureScope) (*assessment.Assessment, *evaluationinput.InputSnapshot, evalrun.EvaluationRun) {
	t.Helper()
	now := time.Now()
	a, err := assessment.NewAssessment(7, testee.NewID(8),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "v1"),
		assessment.NewAnswerSheetRef(meta.FromUint64(42)), assessment.NewAdhocOrigin(),
		assessment.WithID(meta.FromUint64(43)))
	if err != nil {
		t.Fatal(err)
	}
	input := &evaluationinput.InputSnapshot{Model: &evaluationinput.ModelSnapshot{Code: scope.ModelCode}}
	run := evalrun.NewEvaluationRun(43)
	if err := run.Start(now); err != nil {
		t.Fatal(err)
	}
	if err := run.AttachInputSnapshot("frozen-input-ref"); err != nil {
		t.Fatal(err)
	}
	return a, input, run
}

func TestEvaluationAcceptanceClaimRestartProcess(t *testing.T) {
	raw := os.Getenv("QS_M7_EVALUATION_SCOPE")
	if raw == "" {
		return
	}
	var scope AcceptanceFailureScope
	if err := json.Unmarshal([]byte(raw), &scope); err != nil {
		t.Fatal(err)
	}
	db := mongodbtest.ProcessDatabase(t)
	gate, err := NewAcceptanceFailureGate(db.Collection("evaluation_acceptance_failure_claims"), scope)
	if err != nil {
		t.Fatal(err)
	}
	a, input, run := acceptanceEvaluationAttempt(t, scope)
	if os.Getenv("QS_M7_CLAIM_PHASE") == "initial" {
		claims := db.Collection("evaluation_acceptance_failure_claims")
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
				claimed, err := gate.TryFail(t.Context(), a, input, &run)
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
	if claimed, err := gate.TryFail(t.Context(), a, input, &run); err != nil || claimed {
		t.Fatalf("restarted process claim=%t err=%v", claimed, err)
	}
	if count, err := db.Collection("evaluation_acceptance_failure_claims").CountDocuments(t.Context(), bson.M{}); err != nil || count != 1 {
		t.Fatalf("persisted claims=%d err=%v", count, err)
	}
}
