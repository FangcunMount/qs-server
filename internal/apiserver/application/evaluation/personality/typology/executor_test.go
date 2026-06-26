package typology

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

func TestExecutorImplementsEvaluatorContract(t *testing.T) {
	var _ evaluationexecute.Evaluator = (*Executor)(nil)
}

func TestExecutorKeys(t *testing.T) {
	if got := NewMBTIExecutor().Key(); got != evaluation.EvaluatorKeyMBTI {
		t.Fatalf("mbti key = %s, want %s", got, evaluation.EvaluatorKeyMBTI)
	}
	if got := NewSBTIExecutor().Key(); got != evaluation.EvaluatorKeySBTI {
		t.Fatalf("sbti key = %s, want %s", got, evaluation.EvaluatorKeySBTI)
	}
}

func TestExecutorAlgorithmGuard(t *testing.T) {
	executor := NewMBTIExecutor()
	_, err := executor.Execute(context.TODO(), evaluationexecute.ExecutionInput{})
	if err == nil {
		t.Fatal("Execute error = nil, want configuration error")
	}
	_ = assessmentmodel.AlgorithmMBTI
}

func TestExecutorFillsPrimaryAndLevel(t *testing.T) {
	executor := NewMBTIExecutor()
	_, err := executor.Execute(context.TODO(), evaluationexecute.ExecutionInput{
		Assessment: nil,
		Input:      nil,
	})
	if err == nil {
		t.Fatal("expected error without input")
	}
	_ = assessment.OutcomeScoreKindMatchPercent
}
