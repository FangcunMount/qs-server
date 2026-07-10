package modelcatalog

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/taskperformance"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/typology"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type typologyCommandStub struct {
	createCalled bool
}

func (s *typologyCommandStub) List(context.Context, typology.ListInput) (*typology.ModelListResult, error) {
	return nil, nil
}

func (s *typologyCommandStub) Create(_ context.Context, input typology.CreateInput) (*typology.ModelSummary, error) {
	s.createCalled = true
	return &typology.ModelSummary{
		Code:  input.Code,
		Kind:  typology.KindTypology,
		Title: input.Title,
	}, nil
}

func (s *typologyCommandStub) Get(_ context.Context, modelCode string) (*typology.ModelSummary, error) {
	if modelCode == "personality_demo" {
		return &typology.ModelSummary{Code: modelCode, Kind: typology.KindTypology}, nil
	}
	return nil, stderrors.New("not found")
}

func (s *typologyCommandStub) UpdateBasicInfo(context.Context, typology.UpdateBasicInfoInput) (*typology.ModelSummary, error) {
	return nil, nil
}

func (s *typologyCommandStub) Delete(context.Context, string) error { return nil }

func (s *typologyCommandStub) BindQuestionnaire(_ context.Context, input typology.BindQuestionnaireInput) (*typology.QuestionnaireBindingResult, error) {
	return &typology.QuestionnaireBindingResult{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	}, nil
}

func (s *typologyCommandStub) GetQuestionnaire(context.Context, string) (*typology.QuestionnaireBindingResult, error) {
	return nil, nil
}

func (s *typologyCommandStub) GetDefinition(context.Context, string) (*typology.DefinitionResult, error) {
	return nil, nil
}

func (s *typologyCommandStub) UpdateDefinition(context.Context, string, typology.DefinitionInput) (*typology.DefinitionResult, error) {
	return nil, nil
}

func (s *typologyCommandStub) Validate(context.Context, string) (*typology.ValidationResult, error) {
	return nil, nil
}

func (s *typologyCommandStub) PreviewReport(context.Context, string, json.RawMessage) (*typology.PreviewReportResult, error) {
	return nil, nil
}

func (s *typologyCommandStub) Publish(context.Context, string) (*typology.ModelSummary, error) {
	return nil, nil
}

func (s *typologyCommandStub) Unpublish(context.Context, string) (*typology.ModelSummary, error) {
	return nil, nil
}

func (s *typologyCommandStub) Archive(context.Context, string) (*typology.ModelSummary, error) {
	return nil, nil
}

type cognitiveCommandStub struct {
	createCalled        bool
	lastDefinitionInput *taskperformance.DefinitionInput
}

func (s *cognitiveCommandStub) List(context.Context, taskperformance.ListInput) (*taskperformance.ModelListResult, error) {
	return nil, nil
}

func (s *cognitiveCommandStub) Create(_ context.Context, input taskperformance.CreateInput) (*taskperformance.ModelSummary, error) {
	s.createCalled = true
	return &taskperformance.ModelSummary{Code: input.Code, Kind: taskperformance.KindCognitive, Title: input.Title, Status: "draft"}, nil
}

func (s *cognitiveCommandStub) Get(_ context.Context, modelCode string) (*taskperformance.ModelSummary, error) {
	if strings.Contains(modelCode, "cognitive") || strings.Contains(modelCode, "COG") {
		return &taskperformance.ModelSummary{Code: modelCode, Kind: taskperformance.KindCognitive}, nil
	}
	return nil, stderrors.New("not found")
}

func (s *cognitiveCommandStub) UpdateBasicInfo(context.Context, taskperformance.UpdateBasicInfoInput) (*taskperformance.ModelSummary, error) {
	return nil, nil
}

func (s *cognitiveCommandStub) Delete(context.Context, string) error { return nil }

func (s *cognitiveCommandStub) BindQuestionnaire(_ context.Context, input taskperformance.BindQuestionnaireInput) (*taskperformance.QuestionnaireBindingResult, error) {
	return &taskperformance.QuestionnaireBindingResult{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	}, nil
}

func (s *cognitiveCommandStub) GetDefinition(context.Context, string) (*taskperformance.DefinitionResult, error) {
	return nil, nil
}

func (s *cognitiveCommandStub) UpdateDefinition(_ context.Context, modelCode string, input taskperformance.DefinitionInput) (*taskperformance.DefinitionResult, error) {
	s.lastDefinitionInput = &input
	return &taskperformance.DefinitionResult{Payload: input.Payload}, nil
}

func (s *cognitiveCommandStub) Publish(_ context.Context, modelCode string) (*taskperformance.ModelSummary, error) {
	return &taskperformance.ModelSummary{Code: modelCode, Kind: taskperformance.KindCognitive, Status: "published"}, nil
}

func (s *cognitiveCommandStub) Unpublish(context.Context, string) (*taskperformance.ModelSummary, error) {
	return nil, nil
}

func (s *cognitiveCommandStub) Archive(context.Context, string) (*taskperformance.ModelSummary, error) {
	return nil, nil
}

type behavioralRatingCommandStub struct {
	createCalled        bool
	lastDefinitionInput *norming.DefinitionInput
}

func (s *behavioralRatingCommandStub) List(context.Context, norming.ListInput) (*norming.ModelListResult, error) {
	return nil, nil
}

func (s *behavioralRatingCommandStub) Create(_ context.Context, input norming.CreateInput) (*norming.ModelSummary, error) {
	s.createCalled = true
	return &norming.ModelSummary{Code: input.Code, Kind: norming.KindBehavioralRating, Title: input.Title, Status: "draft"}, nil
}

func (s *behavioralRatingCommandStub) Get(_ context.Context, modelCode string) (*norming.ModelSummary, error) {
	if strings.Contains(modelCode, "behavioral") || strings.Contains(modelCode, "BR-") {
		return &norming.ModelSummary{Code: modelCode, Kind: norming.KindBehavioralRating}, nil
	}
	return nil, stderrors.New("not found")
}

func (s *behavioralRatingCommandStub) UpdateBasicInfo(context.Context, norming.UpdateBasicInfoInput) (*norming.ModelSummary, error) {
	return nil, nil
}

func (s *behavioralRatingCommandStub) Delete(context.Context, string) error { return nil }

func (s *behavioralRatingCommandStub) BindQuestionnaire(_ context.Context, input norming.BindQuestionnaireInput) (*norming.QuestionnaireBindingResult, error) {
	return &norming.QuestionnaireBindingResult{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	}, nil
}

func (s *behavioralRatingCommandStub) GetDefinition(context.Context, string) (*norming.DefinitionResult, error) {
	return nil, nil
}

func (s *behavioralRatingCommandStub) UpdateDefinition(_ context.Context, modelCode string, input norming.DefinitionInput) (*norming.DefinitionResult, error) {
	s.lastDefinitionInput = &input
	return &norming.DefinitionResult{PayloadFormat: domain.PayloadFormatBehavioralRatingDefaultV1, Payload: input.Payload}, nil
}

func (s *behavioralRatingCommandStub) Publish(_ context.Context, modelCode string) (*norming.ModelSummary, error) {
	return &norming.ModelSummary{Code: modelCode, Kind: norming.KindBehavioralRating, Status: "published"}, nil
}

func (s *behavioralRatingCommandStub) Unpublish(context.Context, string) (*norming.ModelSummary, error) {
	return nil, nil
}

func (s *behavioralRatingCommandStub) Archive(context.Context, string) (*norming.ModelSummary, error) {
	return nil, nil
}

type personalityQRCodeGeneratorStub struct {
	url string
}

func (s *personalityQRCodeGeneratorStub) GenerateQuestionnaireQRCode(context.Context, string, string) (string, error) {
	return "", nil
}

func (s *personalityQRCodeGeneratorStub) GenerateScaleQRCode(context.Context, string) (string, error) {
	return "", nil
}

func (s *personalityQRCodeGeneratorStub) GenerateAssessmentEntryQRCode(context.Context, string) (string, error) {
	return "", nil
}

func (s *personalityQRCodeGeneratorStub) GeneratePersonalityAssessmentQRCode(_ context.Context, modelCode string) (string, error) {
	if s.url != "" {
		return s.url, nil
	}
	return "https://example.com/qrcodes/personality_" + modelCode + ".png", nil
}

func TestCreateRequiresKind(t *testing.T) {
	svc := NewService(Dependencies{
		TypologyCommand: &typologyCommandStub{},
	})

	_, err := svc.Create(context.Background(), CreateModelDTO{Title: "No Kind"})
	if err == nil {
		t.Fatal("Create() error = nil, want invalid argument")
	}
	if !cberrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("Create() code = %v, want ErrInvalidArgument", cberrors.ParseCoder(err))
	}
}

func TestCreatePersonalityUsesTypologyCommand(t *testing.T) {
	personalityStub := &typologyCommandStub{}
	svc := NewService(Dependencies{
		TypologyCommand: personalityStub,
	})

	result, err := svc.Create(context.Background(), CreateModelDTO{
		Kind:  KindTypology,
		Code:  "personality_create",
		Title: "Personality",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !personalityStub.createCalled {
		t.Fatal("personality command Create was not called")
	}
	if result.Kind != KindTypology {
		t.Fatalf("result kind = %s, want %s", result.Kind, KindTypology)
	}
}

func TestUpdateBehavioralDefinitionMaterializesLegacyPayloadAtGateway(t *testing.T) {
	t.Parallel()

	command := &behavioralRatingCommandStub{}
	svc := NewService(Dependencies{NormingCommand: command})
	payload := json.RawMessage(`{
		"dimensions":[{"code":"gec","title":"GEC"}],
		"brief2":{
			"primary_dimension_code":"gec",
			"norm_table_version":"2024",
			"norms":[{"factor_code":"gec"}]
		}
	}`)
	result, err := svc.UpdateDefinition(context.Background(), "behavioral_demo", DefinitionDTO{
		Kind:    KindBehavioralRating,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	if result == nil || string(result.Payload) != string(payload) {
		t.Fatalf("result payload = %q, want %q", result.Payload, payload)
	}
	input := command.lastDefinitionInput
	if input == nil || input.DefinitionV2 == nil {
		t.Fatalf("definition input = %#v", input)
	}
	if len(input.DefinitionV2.Measure.Factors) != 1 || input.DefinitionV2.Measure.Factors[0].Code != "gec" {
		t.Fatalf("measure = %#v", input.DefinitionV2.Measure)
	}
	if len(input.Norms) != 1 || input.Norms[0].TableVersion != "2024" {
		t.Fatalf("norms = %#v", input.Norms)
	}
}

func TestUpdateCognitiveDefinitionMaterializesLegacyPayloadAtGateway(t *testing.T) {
	t.Parallel()

	command := &cognitiveCommandStub{}
	svc := NewService(Dependencies{TaskPerformanceCommand: command})
	payload := json.RawMessage(`{
		"dimensions":[{"code":"total","title":"Total"}],
		"spm":{"item_set_codes":["total"],"norm_table_version":"spm-2026"}
	}`)
	result, err := svc.UpdateDefinition(context.Background(), "cognitive_demo", DefinitionDTO{
		Kind:    KindCognitive,
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("UpdateDefinition: %v", err)
	}
	if result == nil || string(result.Payload) != string(payload) {
		t.Fatalf("result payload = %q, want %q", result.Payload, payload)
	}
	input := command.lastDefinitionInput
	if input == nil || input.DefinitionV2 == nil {
		t.Fatalf("definition input = %#v", input)
	}
	if len(input.DefinitionV2.Calibration.NormRefs) != 1 || input.DefinitionV2.Calibration.NormRefs[0].NormTableVersion != "spm-2026" {
		t.Fatalf("calibration = %#v", input.DefinitionV2.Calibration)
	}
}

func TestGetQRCodeDispatchesByKind(t *testing.T) {
	personalityStub := &typologyCommandStub{}
	svc := NewService(Dependencies{
		TypologyCommand:    personalityStub,
		RawQRCodeGenerator: &personalityQRCodeGeneratorStub{},
	})

	t.Run("typology model", func(t *testing.T) {
		got, err := svc.GetQRCode(context.Background(), "personality_demo")
		if err != nil {
			t.Fatalf("GetQRCode() error = %v", err)
		}
		want := "https://example.com/qrcodes/personality_personality_demo.png"
		if got != want {
			t.Fatalf("GetQRCode() = %q, want %q", got, want)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		_, err := svc.GetQRCode(context.Background(), "missing_model")
		if err == nil {
			t.Fatal("GetQRCode() error = nil, want not found")
		}
		if !cberrors.IsCode(err, code.ErrMedicalScaleNotFound) {
			t.Fatalf("GetQRCode() code = %v, want ErrMedicalScaleNotFound", cberrors.ParseCoder(err))
		}
	})
}

func TestGetPersonalityQRCodeFallsBackToEntryURL(t *testing.T) {
	svc := NewService(Dependencies{
		TypologyCommand: &typologyCommandStub{},
	})

	got, err := svc.GetQRCode(context.Background(), "personality_demo")
	if err != nil {
		t.Fatalf("GetQRCode() error = %v", err)
	}
	if got != "/personality/assessment/personality_demo" {
		t.Fatalf("GetQRCode() = %q, want entry url fallback", got)
	}
}
