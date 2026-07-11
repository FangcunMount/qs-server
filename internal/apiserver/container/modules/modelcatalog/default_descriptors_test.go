package modelcatalog

import (
	"context"
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	reportmaterialize "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/materialize"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

func TestDefaultEvaluationDescriptorsIncludeScaleAndTypologyModules(t *testing.T) {
	t.Parallel()

	descs := DefaultEvaluationDescriptors()
	if len(descs) != 4 {
		t.Fatalf("descriptor count = %d, want 4 (scale + configured typology + behavioral_rating + cognitive)", len(descs))
	}
	if descs[0].Kind != evaldomain.ModelKindScale {
		t.Fatalf("first descriptor kind = %s, want scale", descs[0].Kind)
	}
	if descs[1].Algorithm != modelcatalog.AlgorithmPersonalityTypology {
		t.Fatalf("configured typology algorithm = %#v", descs[1].Algorithm)
	}
	if descs[2].Kind != evaldomain.ModelKindBehavioralRating {
		t.Fatalf("behavioral_rating kind = %#v", descs[2].Kind)
	}
	if descs[3].Kind != evaldomain.ModelKindCognitive {
		t.Fatalf("cognitive kind = %#v", descs[3].Kind)
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

	descs := DefaultEvaluationDescriptors()
	builders, err := reportmaterialize.ReportBuilders(descs, report.NewDefaultInterpretReportBuilder(nil))
	if err != nil {
		t.Fatalf("ReportBuilders: %v", err)
	}
	providers, err := evaluationinputInfra.MaterializeInputProviders(descs, evaluationinputInfra.InputProviderDeps{
		ScaleCatalog:            evalFakeScaleCatalog{},
		TypologyCatalog:         evalFakeTypologyCatalogPort{},
		BehavioralRatingCatalog: evalFakeBehavioralRatingCatalog{},
		CognitiveCatalog:        evalFakeCognitiveCatalog{},
		AnswerSheets:            evalFakeAnswerSheetReader{},
		Questionnaires:          evalFakeQuestionnaireReader{},
	})
	if err != nil {
		t.Fatalf("MaterializeInputProviders: %v", err)
	}
	if err := evaluation.AssertExecutionPathParity(descs, providers); err != nil {
		t.Fatalf("AssertExecutionPathParity: %v", err)
	}
	for i, desc := range descs {
		want, _ := evaldomain.ExecutionPathForDescriptor(desc)
		got, pathErr := interpretationreporting.ExecutionPathForReportBuilder(builders[i])
		if pathErr != nil || got != want {
			t.Fatalf("report builder path[%d] = %s, want %s (err=%v)", i, got, want, pathErr)
		}
	}
}

func TestRuntimeDescriptorRegistryResolvesLegacyTypologyKeysViaFamilyDescriptor(t *testing.T) {
	t.Parallel()

	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatalf("DefaultRuntimeDescriptorRegistry() error = %v", err)
	}
	for _, legacyKey := range evaldomain.PersonalityTypologyLegacyIdentities() {
		got, err := registry.Resolve(evalpipeline.ModelRoute{Kind: legacyKey.Kind, SubKind: legacyKey.SubKind, Algorithm: legacyKey.Algorithm})
		if err != nil {
			t.Fatalf("Resolve(%s): %v", legacyKey, err)
		}
		if got.AlgorithmFamily != modelcatalog.AlgorithmFamilyFactorClassification {
			t.Fatalf("resolved family = %s, want factor_classification", got.AlgorithmFamily)
		}
	}

	descs := DefaultEvaluationDescriptors()
	builders, err := reportmaterialize.ReportBuilders(descs, report.NewDefaultInterpretReportBuilder(nil))
	if err != nil {
		t.Fatalf("ReportBuilders: %v", err)
	}
	reportRegistry, err := interpretationreporting.NewReportBuilderRegistry(builders...)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	for _, legacyKey := range evaldomain.PersonalityTypologyLegacyIdentities() {
		if _, err := reportRegistry.Resolve(legacyKey, report.ReportTypeStandard); err != nil {
			t.Fatalf("Resolve report(%s): %v", legacyKey, err)
		}
	}

	providers, err := evaluationinputInfra.MaterializeInputProviders(descs, evaluationinputInfra.InputProviderDeps{
		ScaleCatalog:            evalFakeScaleCatalog{},
		TypologyCatalog:         evalFakeTypologyCatalogPort{},
		BehavioralRatingCatalog: evalFakeBehavioralRatingCatalog{},
		CognitiveCatalog:        evalFakeCognitiveCatalog{},
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
	for _, legacyKey := range evaldomain.PersonalityTypologyLegacyIdentities() {
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

type evalFakeCognitiveCatalog struct{}

func (evalFakeCognitiveCatalog) GetCognitiveByRef(context.Context, port.ModelRef) (*taskperfsnapshot.Snapshot, error) {
	return &taskperfsnapshot.Snapshot{}, nil
}

func (evalFakeCognitiveCatalog) FindCognitiveByQuestionnaire(context.Context, string, string) (*taskperfsnapshot.Snapshot, error) {
	return &taskperfsnapshot.Snapshot{}, nil
}

type evalFakeAnswerSheetReader struct{}

func (evalFakeAnswerSheetReader) GetAnswerSheet(context.Context, uint64) (*port.AnswerSheetSnapshot, error) {
	return &port.AnswerSheetSnapshot{}, nil
}

type evalFakeQuestionnaireReader struct{}

func (evalFakeQuestionnaireReader) GetQuestionnaire(context.Context, string, string) (*port.QuestionnaireSnapshot, error) {
	return &port.QuestionnaireSnapshot{}, nil
}
