package characterization_test

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// V1 contract: answersheet-like path — submitted assessment resolves input, executes, persists report outcome.
func TestV1PipelineScaleSubmitToInterpretedOutcome(t *testing.T) {
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
	result := writer.outcome.Execution
	if result == nil {
		t.Fatal("expected assessment outcome after pipeline")
	}
	if result.Primary == nil || result.Primary.Value != 5 || result.Level == nil || result.Level.Code != string(assessment.RiskLevelLow) {
		t.Fatalf("outcome = primary:%v level:%v, want 5/low", result.Primary, result.Level)
	}
}

// V1 contract: personality path completes with interpreted status and primary label.
func TestV1PipelineMBTISubmitToInterpretedOutcome(t *testing.T) {
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
	result := writer.outcome.Execution
	if result == nil || result.Summary.PrimaryLabel != "INTJ" {
		t.Fatalf("PrimaryLabel = %q, want INTJ", result.Summary.PrimaryLabel)
	}
}
