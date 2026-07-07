package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	factorclassification "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_classification"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_norm"
	factorscoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/factor_scoring"
	taskperformance "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/task_performance"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

func newV1EvaluatorRegistry(t *testing.T) evaluationexecute.EvaluatorRegistry {
	t.Helper()
	configured, err := factorclassification.NewConfiguredTypologyExecutor()
	if err != nil {
		t.Fatalf("NewConfiguredTypologyExecutor: %v", err)
	}
	registry, err := evaluationexecute.NewEvaluatorRegistry(
		factorscoring.NewExecutor(nil),
		factornorm.NewExecutor(nil),
		taskperformance.NewExecutor(nil),
		configured,
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
	input := &charInputResolver{snapshot: scaleInputSnapshot()}
	svc, capture := newV1RecordingExecuteService(t, a, input)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if capture.interpretationCalls != 1 {
		t.Fatalf("interpretation calls = %d, want 1", capture.interpretationCalls)
	}
	result := capture.outcome.Execution
	if result == nil {
		t.Fatal("expected assessment outcome")
	}
	if result.Primary == nil || result.Primary.Value != 5 || result.Level == nil || result.Level.Code != string(assessment.RiskLevelLow) {
		t.Fatalf("outcome = primary:%v level:%v, want 5/low", result.Primary, result.Level)
	}
	if input.lastRef.ModelRef.Kind != "scale" || input.lastRef.ModelRef.Code != "S-001" {
		t.Fatalf("input ref = %#v", input.lastRef)
	}
}

// V1 contract: execute service dispatches legacy MBTI kind to typology/mbti evaluator.
func TestV1ExecuteServiceDispatchesMBTIByLegacyKind(t *testing.T) {
	a := submittedMBTIAssessment(t)
	input := &charInputResolver{snapshot: mbtiInputSnapshot()}
	svc, capture := newV1RecordingExecuteService(t, a, input)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if capture.interpretationCalls != 1 {
		t.Fatalf("interpretation calls = %d, want 1", capture.interpretationCalls)
	}
	result := capture.outcome.Execution
	if result == nil || result.ModelRef.Kind() != assessment.EvaluationModelKindPersonality {
		t.Fatalf("model kind = %s, want personality", result.ModelRef.Kind())
	}
	if result.Summary.PrimaryLabel != "INTJ" {
		t.Fatalf("PrimaryLabel = %q, want INTJ", result.Summary.PrimaryLabel)
	}
}

// V1 contract: execute service dispatches legacy SBTI kind to typology/sbti evaluator.
func TestV1ExecuteServiceDispatchesSBTIByLegacyKind(t *testing.T) {
	a := submittedSBTIAssessment(t)
	input := &charInputResolver{snapshot: sbtiInputSnapshot()}
	svc, capture := newV1RecordingExecuteService(t, a, input)

	if err := svc.Evaluate(context.Background(), a.ID().Uint64()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if capture.interpretationCalls != 1 {
		t.Fatalf("interpretation calls = %d, want 1", capture.interpretationCalls)
	}
	result := capture.outcome.Execution
	if result == nil || result.ModelRef.Kind() != assessment.EvaluationModelKindPersonality {
		t.Fatalf("model kind = %s, want personality", result.ModelRef.Kind())
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
		factorscoring.NewExecutor(nil),
	)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	svc := evaluationexecute.NewService(
		repo,
		input,
		evaluationexecute.WithEvaluatorRegistry(registry),
		evaluationexecute.WithScoringWriter(charRecordingScoring{}),
		evaluationexecute.WithInterpretationService(&charRecordingInterpretation{cap: &charSplitPhaseCapture{}}),
	)
	err = svc.Evaluate(context.Background(), a.ID().Uint64())
	if err == nil {
		t.Fatal("Evaluate error = nil, want unsupported model key")
	}
	if !a.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", a.Status())
	}
}
