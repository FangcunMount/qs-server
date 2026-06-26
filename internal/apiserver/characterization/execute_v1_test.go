package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	typologyeval "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology"
	evaluationscale "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

func newV1EvaluatorRegistry(t *testing.T) evaluationexecute.EvaluatorRegistry {
	t.Helper()
	registry, err := evaluationexecute.NewEvaluatorRegistry(
		evaluationscale.NewExecutor(nil),
		typologyeval.NewSBTIExecutor(),
		typologyeval.NewMBTIExecutor(),
	)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	return registry
}

// V1 contract: execute service resolves EvaluatorKey and dispatches scale evaluator;
// writer receives total=5 risk=low result.
func TestV1ExecuteServiceDispatchesScaleByEvaluatorKey(t *testing.T) {
	a := submittedScaleAssessment(t)
	repo := &charAssessmentRepo{assessment: a}
	input := &charInputResolver{snapshot: scaleInputSnapshot()}
	writer := &charResultWriter{}

	svc := evaluationexecute.NewService(
		repo,
		input,
		writer,
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
	)
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls = %d, want 1", writer.calls)
	}
	result := writer.outcome.LegacyResult()
	if result == nil {
		t.Fatal("expected evaluation result")
	}
	if result.TotalScore != 5 || result.RiskLevel != assessment.RiskLevelLow {
		t.Fatalf("result = score:%.1f risk:%s, want 5/low", result.TotalScore, result.RiskLevel)
	}
	if input.lastRef.ModelRef.Kind != "scale" || input.lastRef.ModelRef.Code != "S-001" {
		t.Fatalf("input ref = %#v", input.lastRef)
	}
}

// V1 contract: execute service dispatches legacy MBTI kind to typology/mbti evaluator.
func TestV1ExecuteServiceDispatchesMBTIByLegacyKind(t *testing.T) {
	a := submittedMBTIAssessment(t)
	repo := &charAssessmentRepo{assessment: a}
	input := &charInputResolver{snapshot: mbtiInputSnapshot()}
	writer := &charResultWriter{}

	svc := evaluationexecute.NewService(
		repo,
		input,
		writer,
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
	)
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls = %d, want 1", writer.calls)
	}
	result := writer.outcome.LegacyResult()
	if result == nil || result.ModelRef.Kind() != assessment.EvaluationModelKindPersonality {
		t.Fatalf("model kind = %s, want mbti", result.ModelRef.Kind())
	}
	if result.Summary.PrimaryLabel != "INTJ" {
		t.Fatalf("PrimaryLabel = %q, want INTJ", result.Summary.PrimaryLabel)
	}
}

// V1 contract: execute service dispatches legacy SBTI kind to typology/sbti evaluator.
func TestV1ExecuteServiceDispatchesSBTIByLegacyKind(t *testing.T) {
	a := submittedSBTIAssessment(t)
	repo := &charAssessmentRepo{assessment: a}
	input := &charInputResolver{snapshot: sbtiInputSnapshot()}
	writer := &charResultWriter{}

	svc := evaluationexecute.NewService(
		repo,
		input,
		writer,
		evaluationexecute.WithEvaluatorRegistry(newV1EvaluatorRegistry(t)),
	)
	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if writer.calls != 1 {
		t.Fatalf("writer calls = %d, want 1", writer.calls)
	}
	result := writer.outcome.LegacyResult()
	if result == nil || result.ModelRef.Kind() != assessment.EvaluationModelKindPersonality {
		t.Fatalf("model kind = %s, want sbti", result.ModelRef.Kind())
	}
	if result.Summary.PrimaryLabel != "HIGH" {
		t.Fatalf("PrimaryLabel = %q, want HIGH", result.Summary.PrimaryLabel)
	}
	if result.Summary.Score == nil || *result.Summary.Score != 100 {
		t.Fatalf("Score = %v, want 100", result.Summary.Score)
	}
}

// V1 contract: evaluator registry rejects unknown v2 keys.
func TestV1ExecuteServiceRejectsUnknownEvaluatorKey(t *testing.T) {
	a := submittedMBTIAssessment(t)
	repo := &charAssessmentRepo{assessment: a}
	input := &charInputResolver{snapshot: mbtiInputSnapshot()}

	registry, err := evaluationexecute.NewEvaluatorRegistry(
		evaluationscale.NewExecutor(nil),
	)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	svc := evaluationexecute.NewService(
		repo,
		input,
		&charResultWriter{},
		evaluationexecute.WithEvaluatorRegistry(registry),
	)
	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if err == nil {
		t.Fatal("Evaluate error = nil, want unsupported model key")
	}
	if !a.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", a.Status())
	}
}
