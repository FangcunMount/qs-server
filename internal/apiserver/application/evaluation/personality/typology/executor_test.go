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

func TestExecutorLegacyKinds(t *testing.T) {
	if got := NewMBTIExecutor().Kind(); got != assessment.EvaluationModelKindPersonality {
		t.Fatalf("mbti kind = %s, want mbti", got)
	}
	if got := NewSBTIExecutor().Kind(); got != assessment.EvaluationModelKindPersonality {
		t.Fatalf("sbti kind = %s, want sbti", got)
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
