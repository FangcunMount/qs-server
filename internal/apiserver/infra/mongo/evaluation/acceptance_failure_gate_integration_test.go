//go:build integration && reliable_messaging_m4

package evaluation

import (
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
	claims := db.Collection("evaluation_acceptance_failure_claims")
	first, err := NewAcceptanceFailureGate(claims, scope)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewAcceptanceFailureGate(claims, scope)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := first.TryFail(t.Context(), a, input, &run)
	if err != nil || !claimed {
		t.Fatalf("first claim=%t err=%v", claimed, err)
	}
	claimed, err = second.TryFail(t.Context(), a, input, &run)
	if err != nil || claimed {
		t.Fatalf("restarted gate claim=%t err=%v", claimed, err)
	}
	if count, err := claims.CountDocuments(t.Context(), bson.M{}); err != nil || count != 1 {
		t.Fatalf("persisted claims=%d err=%v", count, err)
	}
}
