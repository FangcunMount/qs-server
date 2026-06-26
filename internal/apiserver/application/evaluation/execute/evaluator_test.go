package execute

import (
	"context"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

type evaluatorStub struct {
	key     evaluation.EvaluatorKey
	kind    assessment.EvaluationModelKind
	execute func(context.Context, ExecutionInput) (*assessment.EvaluationResult, error)
}

func (e evaluatorStub) Key() evaluation.EvaluatorKey {
	if !e.key.IsZero() {
		return e.key
	}
	if key, ok := evaluation.EvaluatorKeyFromLegacyKind(assessmentmodel.Kind(e.kind)); ok {
		return key
	}
	return evaluation.EvaluatorKey{}
}

func (e evaluatorStub) Kind() assessment.EvaluationModelKind {
	return e.kind
}

func (e evaluatorStub) Execute(ctx context.Context, input ExecutionInput) (*assessment.EvaluationResult, error) {
	if e.execute != nil {
		return e.execute(ctx, input)
	}
	return assessment.NewEvaluationResult(0, assessment.RiskLevelNone, "", "", nil), nil
}

func TestEvaluatorRegistryResolvesRegisteredEvaluator(t *testing.T) {
	scaleEvaluator := evaluatorStub{kind: assessment.EvaluationModelKindScale}
	registry, err := NewEvaluatorRegistry(scaleEvaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}

	got, err := registry.Resolve(evaluation.EvaluatorKeyScaleDefault)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.Kind() != assessment.EvaluationModelKindScale {
		t.Fatalf("resolved kind = %s, want scale", got.Kind())
	}
}

func TestEvaluatorRegistryResolveLegacyKind(t *testing.T) {
	registry, err := NewEvaluatorRegistry(evaluatorStub{
		key:  evaluation.EvaluatorKeyMBTI,
		kind: assessment.EvaluationModelKindPersonality,
	})
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}
	got, err := registry.ResolveLegacyKind(assessmentmodel.KindMBTIMigration)
	if err != nil {
		t.Fatalf("ResolveLegacyKind returned error: %v", err)
	}
	if got.Kind() != assessment.EvaluationModelKindPersonality {
		t.Fatalf("resolved kind = %s, want mbti", got.Kind())
	}
}

func TestEvaluatorRegistryRejectsDuplicateKind(t *testing.T) {
	_, err := NewEvaluatorRegistry(
		evaluatorStub{kind: assessment.EvaluationModelKindScale},
		evaluatorStub{kind: assessment.EvaluationModelKindScale},
	)
	if err == nil {
		t.Fatal("NewEvaluatorRegistry error = nil, want duplicate kind error")
	}
}

func TestEvaluatorRegistryRejectsUnknownKind(t *testing.T) {
	registry, err := NewEvaluatorRegistry(evaluatorStub{kind: assessment.EvaluationModelKindScale})
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}

	_, err = registry.Resolve(evaluation.EvaluatorKeyMBTI)
	if err == nil {
		t.Fatal("Resolve error = nil, want unsupported model kind")
	}
}

func TestEvaluatorContractDoesNotImportLegacyPipeline(t *testing.T) {
	file := filepath.Join("evaluator.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	for _, imp := range parsed.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if strings.Contains(path, "/application/evaluation/engine/pipeline") {
			t.Fatalf("evaluator contract must not import legacy pipeline package: %s", path)
		}
	}
}
