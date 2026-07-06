package modelcatalog

import (
	"context"
	"testing"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/snapshot"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleengine"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestDefaultEvaluationDescriptorsIncludeScaleAndTypologyModules(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	if len(descs) != 3 {
		t.Fatalf("descriptor count = %d, want 3 (scale + configured typology + behavioral_rating)", len(descs))
	}
	if descs[0].Kind != evaldomain.ModelKindScale {
		t.Fatalf("first descriptor kind = %s, want scale", descs[0].Kind)
	}
	if descs[1].Key != evaldomain.EvaluatorKeyPersonalityTypology {
		t.Fatalf("configured typology key = %#v", descs[1].Key)
	}
	if descs[2].Key != evaldomain.EvaluatorKeyBehavioralRatingDefault {
		t.Fatalf("behavioral_rating key = %#v", descs[2].Key)
	}
	typology := evaldomain.TypologyAlgorithms(descs)
	if len(typology) != 1 || typology[0] != modelcatalog.AlgorithmPersonalityTypology {
		t.Fatalf("typology algorithms = %#v", typology)
	}
}

func TestDefaultTypologyRegistryResolvesBuiltInModules(t *testing.T) {
	t.Parallel()

	registry, err := DefaultTypologyRegistry()
	if err != nil {
		t.Fatalf("DefaultTypologyRegistry() error = %v", err)
	}
	if registry.Len() != len(DefaultTypologyModules()) {
		t.Fatalf("registry len = %d, want %d", registry.Len(), len(DefaultTypologyModules()))
	}
}

func TestMaterializeRegistryKeyParity(t *testing.T) {
	t.Parallel()

	registry, err := DefaultTypologyRegistry()
	if err != nil {
		t.Fatalf("DefaultTypologyRegistry() error = %v", err)
	}
	descs := DefaultEvaluationDescriptors()
	wiringDeps := evaluation.WiringDeps{
		ScaleReportBuilder: report.NewDefaultInterpretReportBuilder(nil),
		ScaleScorer:        ruleengine.NewScaleFactorScorer(),
		TypologyRegistry:   registry,
	}
	evaluators, err := evaluation.MaterializeEvaluators(descs, wiringDeps)
	if err != nil {
		t.Fatalf("MaterializeEvaluators: %v", err)
	}
	builders, err := evaluation.MaterializeReportBuilders(descs, wiringDeps)
	if err != nil {
		t.Fatalf("MaterializeReportBuilders: %v", err)
	}
	providers, err := evaluationinputInfra.MaterializeInputProviders(descs, evaluationinputInfra.InputProviderDeps{
		ScaleCatalog:            evalFakeScaleCatalog{},
		TypologyCatalog:         evalFakeTypologyCatalogPort{},
		BehavioralRatingCatalog: evalFakeBehavioralRatingCatalog{},
		AnswerSheets:            evalFakeAnswerSheetReader{},
		Questionnaires:          evalFakeQuestionnaireReader{},
	})
	if err != nil {
		t.Fatalf("MaterializeInputProviders: %v", err)
	}
	if err := evaluation.AssertRegistryKeyParity(descs, evaluators, builders, providers); err != nil {
		t.Fatalf("AssertRegistryKeyParity: %v", err)
	}
}

func TestMaterializedRegistryResolvesLegacyTypologyKeysViaConfiguredDescriptor(t *testing.T) {
	t.Parallel()

	registry, err := DefaultTypologyRegistry()
	if err != nil {
		t.Fatalf("DefaultTypologyRegistry() error = %v", err)
	}
	descs := DefaultEvaluationDescriptors()
	wiringDeps := evaluation.WiringDeps{
		ScaleReportBuilder: report.NewDefaultInterpretReportBuilder(nil),
		ScaleScorer:        ruleengine.NewScaleFactorScorer(),
		TypologyRegistry:   registry,
	}
	evaluators, err := evaluation.MaterializeEvaluators(descs, wiringDeps)
	if err != nil {
		t.Fatalf("MaterializeEvaluators: %v", err)
	}
	evaluatorRegistry, err := evaluationexecute.NewEvaluatorRegistry(evaluators...)
	if err != nil {
		t.Fatalf("NewEvaluatorRegistry: %v", err)
	}
	for _, legacyKey := range evaldomain.PersonalityTypologyLegacyKeys() {
		got, err := evaluatorRegistry.Resolve(legacyKey)
		if err != nil {
			t.Fatalf("Resolve(%s): %v", legacyKey, err)
		}
		if got.Key() != evaldomain.EvaluatorKeyPersonalityTypology {
			t.Fatalf("resolved executor key = %s, want configured typology", got.Key())
		}
	}

	builders, err := evaluation.MaterializeReportBuilders(descs, wiringDeps)
	if err != nil {
		t.Fatalf("MaterializeReportBuilders: %v", err)
	}
	reportRegistry, err := interpretationreporting.NewReportBuilderRegistry(builders...)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	for _, legacyKey := range evaldomain.PersonalityTypologyLegacyKeys() {
		if _, err := reportRegistry.Resolve(legacyKey, report.ReportTypeStandard); err != nil {
			t.Fatalf("Resolve report(%s): %v", legacyKey, err)
		}
	}

	providers, err := evaluationinputInfra.MaterializeInputProviders(descs, evaluationinputInfra.InputProviderDeps{
		ScaleCatalog:            evalFakeScaleCatalog{},
		TypologyCatalog:         evalFakeTypologyCatalogPort{},
		BehavioralRatingCatalog: evalFakeBehavioralRatingCatalog{},
		AnswerSheets:            evalFakeAnswerSheetReader{},
		Questionnaires:          evalFakeQuestionnaireReader{},
	})
	if err != nil {
		t.Fatalf("MaterializeInputProviders: %v", err)
	}
	providerRegistry, err := evaluationinputInfra.NewModelInputProviderRegistry(providers...)
	if err != nil {
		t.Fatalf("NewModelInputProviderRegistry: %v", err)
	}
	for _, legacyKey := range evaldomain.PersonalityTypologyLegacyKeys() {
		if _, err := providerRegistry.Resolve(legacyKey); err != nil {
			t.Fatalf("Resolve provider(%s): %v", legacyKey, err)
		}
	}
}

type evalFakeScaleCatalog struct{}

func (evalFakeScaleCatalog) GetScale(context.Context, string) (*scalesnapshot.ScaleSnapshot, error) {
	return &scalesnapshot.ScaleSnapshot{}, nil
}

func (evalFakeScaleCatalog) GetScaleByRef(context.Context, port.ModelRef) (*scalesnapshot.ScaleSnapshot, error) {
	return &scalesnapshot.ScaleSnapshot{}, nil
}

type evalFakeTypologyCatalogPort struct{}

func (evalFakeTypologyCatalogPort) GetTypologyModelByRef(context.Context, port.ModelRef) (*modeltypology.Payload, error) {
	return &modeltypology.Payload{}, nil
}

func (evalFakeTypologyCatalogPort) FindTypologyModelByQuestionnaire(context.Context, string, string) (*modeltypology.Payload, error) {
	return &modeltypology.Payload{}, nil
}

type evalFakeBehavioralRatingCatalog struct{}

func (evalFakeBehavioralRatingCatalog) GetBehavioralRatingByRef(context.Context, port.ModelRef) (*behavioralsnapshot.Snapshot, error) {
	return &behavioralsnapshot.Snapshot{}, nil
}

func (evalFakeBehavioralRatingCatalog) FindBehavioralRatingByQuestionnaire(context.Context, string, string) (*behavioralsnapshot.Snapshot, error) {
	return &behavioralsnapshot.Snapshot{}, nil
}

type evalFakeAnswerSheetReader struct{}

func (evalFakeAnswerSheetReader) GetAnswerSheet(context.Context, uint64) (*port.AnswerSheetSnapshot, error) {
	return &port.AnswerSheetSnapshot{}, nil
}

type evalFakeQuestionnaireReader struct{}

func (evalFakeQuestionnaireReader) GetQuestionnaire(context.Context, string, string) (*port.QuestionnaireSnapshot, error) {
	return &port.QuestionnaireSnapshot{}, nil
}
