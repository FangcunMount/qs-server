package execute

import (
	"context"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type evaluatorStub struct {
	key     evaluation.EvaluatorKey
	execute func(context.Context, ExecutionInput) (*assessment.AssessmentOutcome, error)
}

func (e evaluatorStub) Key() evaluation.EvaluatorKey {
	return e.key
}

func (e evaluatorStub) Execute(ctx context.Context, input ExecutionInput) (*assessment.AssessmentOutcome, error) {
	if e.execute != nil {
		return e.execute(ctx, input)
	}
	return assessment.NewAssessmentOutcome(
		assessment.EvaluationModelRef{},
		assessment.ResultSummary{},
		assessment.EvaluationDetail{},
	), nil
}

func TestEvaluatorRegistryResolvesRegisteredEvaluator(t *testing.T) {
	scaleEvaluator := evaluatorStub{key: evaluation.EvaluatorKeyScaleDefault}
	registry, err := NewEvaluatorRegistry(scaleEvaluator)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}

	got, err := registry.Resolve(evaluation.EvaluatorKeyScaleDefault)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.Key() != evaluation.EvaluatorKeyScaleDefault {
		t.Fatalf("resolved key = %s, want scale default", got.Key())
	}
}

func TestEvaluatorRegistryResolvesByEvaluatorKey(t *testing.T) {
	registry, err := NewEvaluatorRegistry(evaluatorStub{
		key: evaluation.EvaluatorKeyMBTI,
	})
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}
	got, err := registry.Resolve(evaluation.EvaluatorKeyMBTI)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.Key() != evaluation.EvaluatorKeyMBTI {
		t.Fatalf("resolved key = %s, want mbti", got.Key())
	}
}

func TestEvaluatorRegistryRejectsDuplicateKey(t *testing.T) {
	_, err := NewEvaluatorRegistry(
		evaluatorStub{key: evaluation.EvaluatorKeyScaleDefault},
		evaluatorStub{key: evaluation.EvaluatorKeyScaleDefault},
	)
	if err == nil {
		t.Fatal("NewEvaluatorRegistry error = nil, want duplicate key error")
	}
}

func TestEvaluatorRegistryResolvesLegacyTypologyViaConfiguredKey(t *testing.T) {
	configured := evaluatorStub{key: evaluation.EvaluatorKeyPersonalityTypology}
	registry, err := NewEvaluatorRegistry(configured)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}
	got, err := registry.Resolve(evaluation.EvaluatorKeyMBTI)
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if got.Key() != evaluation.EvaluatorKeyPersonalityTypology {
		t.Fatalf("resolved key = %s, want configured typology", got.Key())
	}
}

func TestResolveEvaluatorKeyPrefersInputAlgorithmWhenAssessmentMissing(t *testing.T) {
	modelRef := assessment.NewEvaluationModelRefByCode(
		assessment.EvaluationModelKindPersonality,
		meta.NewCode("BIG5_IPIP_50"),
		"1.0.0",
		"大五人格",
	)
	a, err := assessment.NewAssessment(
		1,
		meta.FromUint64(2001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("BIG5_IPIP_50"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(5001)),
		assessment.NewAdhocOrigin(),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	input := &evaluationinput.InputSnapshot{
		Model: evaluationinput.NewTypologyModelSnapshot(&modeltypology.Payload{
			Code:      "BIG5_IPIP_50",
			Version:   "1.0.0",
			Algorithm: modelcatalog.AlgorithmBigFive,
			Status:    "published",
		}),
	}
	got := resolveEvaluatorKey(a, input)
	want := evaluation.EvaluatorKeyBigFive
	if got != want {
		t.Fatalf("resolveEvaluatorKey() = %s, want %s", got, want)
	}
}

func TestEvaluatorRegistryRejectsUnknownKey(t *testing.T) {
	registry, err := NewEvaluatorRegistry(evaluatorStub{key: evaluation.EvaluatorKeyScaleDefault})
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry returned error: %v", err)
	}

	_, err = registry.Resolve(evaluation.EvaluatorKey{Kind: modelcatalog.KindCustom})
	if err == nil {
		t.Fatal("Resolve error = nil, want unsupported model key")
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

func TestEvaluatorStubUsesLegacyKindMappingOnlyInTests(t *testing.T) {
	_ = modelcatalog.KindMBTIMigration
}
