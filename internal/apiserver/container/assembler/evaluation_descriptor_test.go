package assembler

import (
	"context"
	"testing"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestMaterializeRegistryKeyParity(t *testing.T) {
	descs := DefaultEvaluationDescriptors()
	deps := DefaultEvaluationWiringDeps(report.NewDefaultInterpretReportBuilder(nil))
	evaluators, err := MaterializeEvaluators(descs, deps)
	if err != nil {
		t.Fatalf("MaterializeEvaluators: %v", err)
	}
	builders, err := MaterializeReportBuilders(descs, deps)
	if err != nil {
		t.Fatalf("MaterializeReportBuilders: %v", err)
	}
	providers, err := evaluationinputInfra.MaterializeInputProviders(descs, evaluationinputInfra.InputProviderDeps{
		ScaleCatalog:    evalFakeScaleCatalog{},
		TypologyCatalog: evalFakeTypologyCatalogPort{},
		AnswerSheets:    evalFakeAnswerSheetReader{},
		Questionnaires:  evalFakeQuestionnaireReader{},
	})
	if err != nil {
		t.Fatalf("MaterializeInputProviders: %v", err)
	}
	if err := AssertRegistryKeyParity(descs, evaluators, builders, providers); err != nil {
		t.Fatalf("AssertRegistryKeyParity: %v", err)
	}
}

func TestDefaultEvaluationDescriptorsIncludeScaleAndTypologyModules(t *testing.T) {
	descs := DefaultEvaluationDescriptors()
	if len(descs) < 3 {
		t.Fatalf("descriptor count = %d, want at least 3", len(descs))
	}
	if descs[0].Kind != evaldomain.ModelKindScale {
		t.Fatalf("first descriptor kind = %s, want scale", descs[0].Kind)
	}
	typology := evaldomain.TypologyAlgorithms(descs)
	if len(typology) != 3 {
		t.Fatalf("typology algorithms = %#v", typology)
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

type evalFakeAnswerSheetReader struct{}

func (evalFakeAnswerSheetReader) GetAnswerSheet(context.Context, uint64) (*port.AnswerSheetSnapshot, error) {
	return &port.AnswerSheetSnapshot{}, nil
}

type evalFakeQuestionnaireReader struct{}

func (evalFakeQuestionnaireReader) GetQuestionnaire(context.Context, string, string) (*port.QuestionnaireSnapshot, error) {
	return &port.QuestionnaireSnapshot{}, nil
}
