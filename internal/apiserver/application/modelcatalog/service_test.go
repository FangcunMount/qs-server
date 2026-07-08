package modelcatalog

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavioral_rating"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/cognitive"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type personalityCommandStub struct {
	createCalled bool
}

func (s *personalityCommandStub) List(context.Context, personality.ListInput) (*personality.ModelListResult, error) {
	return nil, nil
}

func (s *personalityCommandStub) Create(_ context.Context, input personality.CreateInput) (*personality.ModelSummary, error) {
	s.createCalled = true
	return &personality.ModelSummary{
		Code:  input.Code,
		Kind:  personality.KindPersonality,
		Title: input.Title,
	}, nil
}

func (s *personalityCommandStub) Get(_ context.Context, modelCode string) (*personality.ModelSummary, error) {
	if modelCode == "personality_demo" {
		return &personality.ModelSummary{Code: modelCode, Kind: personality.KindPersonality}, nil
	}
	return nil, stderrors.New("not found")
}

func (s *personalityCommandStub) UpdateBasicInfo(context.Context, personality.UpdateBasicInfoInput) (*personality.ModelSummary, error) {
	return nil, nil
}

func (s *personalityCommandStub) Delete(context.Context, string) error { return nil }

func (s *personalityCommandStub) BindQuestionnaire(_ context.Context, input personality.BindQuestionnaireInput) (*personality.QuestionnaireBindingResult, error) {
	return &personality.QuestionnaireBindingResult{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	}, nil
}

func (s *personalityCommandStub) GetQuestionnaire(context.Context, string) (*personality.QuestionnaireBindingResult, error) {
	return nil, nil
}

func (s *personalityCommandStub) GetDefinition(context.Context, string) (*personality.DefinitionResult, error) {
	return nil, nil
}

func (s *personalityCommandStub) UpdateDefinition(context.Context, string, personality.DefinitionInput) (*personality.DefinitionResult, error) {
	return nil, nil
}

func (s *personalityCommandStub) Validate(context.Context, string) (*personality.ValidationResult, error) {
	return nil, nil
}

func (s *personalityCommandStub) PreviewReport(context.Context, string, json.RawMessage) (*personality.PreviewReportResult, error) {
	return nil, nil
}

func (s *personalityCommandStub) Publish(context.Context, string) (*personality.ModelSummary, error) {
	return nil, nil
}

func (s *personalityCommandStub) Unpublish(context.Context, string) (*personality.ModelSummary, error) {
	return nil, nil
}

func (s *personalityCommandStub) Archive(context.Context, string) (*personality.ModelSummary, error) {
	return nil, nil
}

type cognitiveCommandStub struct {
	createCalled bool
}

func (s *cognitiveCommandStub) List(context.Context, cognitive.ListInput) (*cognitive.ModelListResult, error) {
	return nil, nil
}

func (s *cognitiveCommandStub) Create(_ context.Context, input cognitive.CreateInput) (*cognitive.ModelSummary, error) {
	s.createCalled = true
	return &cognitive.ModelSummary{Code: input.Code, Kind: cognitive.KindCognitive, Title: input.Title, Status: "draft"}, nil
}

func (s *cognitiveCommandStub) Get(_ context.Context, modelCode string) (*cognitive.ModelSummary, error) {
	if strings.Contains(modelCode, "cognitive") || strings.Contains(modelCode, "COG") {
		return &cognitive.ModelSummary{Code: modelCode, Kind: cognitive.KindCognitive}, nil
	}
	return nil, stderrors.New("not found")
}

func (s *cognitiveCommandStub) UpdateBasicInfo(context.Context, cognitive.UpdateBasicInfoInput) (*cognitive.ModelSummary, error) {
	return nil, nil
}

func (s *cognitiveCommandStub) Delete(context.Context, string) error { return nil }

func (s *cognitiveCommandStub) BindQuestionnaire(_ context.Context, input cognitive.BindQuestionnaireInput) (*cognitive.QuestionnaireBindingResult, error) {
	return &cognitive.QuestionnaireBindingResult{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	}, nil
}

func (s *cognitiveCommandStub) GetDefinition(context.Context, string) (*cognitive.DefinitionResult, error) {
	return nil, nil
}

func (s *cognitiveCommandStub) UpdateDefinition(_ context.Context, modelCode string, input cognitive.DefinitionInput) (*cognitive.DefinitionResult, error) {
	return &cognitive.DefinitionResult{Payload: input.Payload}, nil
}

func (s *cognitiveCommandStub) Publish(_ context.Context, modelCode string) (*cognitive.ModelSummary, error) {
	return &cognitive.ModelSummary{Code: modelCode, Kind: cognitive.KindCognitive, Status: "published"}, nil
}

func (s *cognitiveCommandStub) Unpublish(context.Context, string) (*cognitive.ModelSummary, error) {
	return nil, nil
}

func (s *cognitiveCommandStub) Archive(context.Context, string) (*cognitive.ModelSummary, error) {
	return nil, nil
}

type behavioralRatingCommandStub struct {
	createCalled bool
}

func (s *behavioralRatingCommandStub) List(context.Context, behavioral_rating.ListInput) (*behavioral_rating.ModelListResult, error) {
	return nil, nil
}

func (s *behavioralRatingCommandStub) Create(_ context.Context, input behavioral_rating.CreateInput) (*behavioral_rating.ModelSummary, error) {
	s.createCalled = true
	return &behavioral_rating.ModelSummary{Code: input.Code, Kind: behavioral_rating.KindBehavioralRating, Title: input.Title, Status: "draft"}, nil
}

func (s *behavioralRatingCommandStub) Get(_ context.Context, modelCode string) (*behavioral_rating.ModelSummary, error) {
	if strings.Contains(modelCode, "behavioral") || strings.Contains(modelCode, "BR-") {
		return &behavioral_rating.ModelSummary{Code: modelCode, Kind: behavioral_rating.KindBehavioralRating}, nil
	}
	return nil, stderrors.New("not found")
}

func (s *behavioralRatingCommandStub) UpdateBasicInfo(context.Context, behavioral_rating.UpdateBasicInfoInput) (*behavioral_rating.ModelSummary, error) {
	return nil, nil
}

func (s *behavioralRatingCommandStub) Delete(context.Context, string) error { return nil }

func (s *behavioralRatingCommandStub) BindQuestionnaire(_ context.Context, input behavioral_rating.BindQuestionnaireInput) (*behavioral_rating.QuestionnaireBindingResult, error) {
	return &behavioral_rating.QuestionnaireBindingResult{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	}, nil
}

func (s *behavioralRatingCommandStub) GetDefinition(context.Context, string) (*behavioral_rating.DefinitionResult, error) {
	return nil, nil
}

func (s *behavioralRatingCommandStub) UpdateDefinition(_ context.Context, modelCode string, input behavioral_rating.DefinitionInput) (*behavioral_rating.DefinitionResult, error) {
	return &behavioral_rating.DefinitionResult{PayloadFormat: domain.PayloadFormatBehavioralRatingDefaultV1, Payload: input.Payload}, nil
}

func (s *behavioralRatingCommandStub) Publish(_ context.Context, modelCode string) (*behavioral_rating.ModelSummary, error) {
	return &behavioral_rating.ModelSummary{Code: modelCode, Kind: behavioral_rating.KindBehavioralRating, Status: "published"}, nil
}

func (s *behavioralRatingCommandStub) Unpublish(context.Context, string) (*behavioral_rating.ModelSummary, error) {
	return nil, nil
}

func (s *behavioralRatingCommandStub) Archive(context.Context, string) (*behavioral_rating.ModelSummary, error) {
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
		PersonalityCommand: &personalityCommandStub{},
	})

	_, err := svc.Create(context.Background(), CreateModelDTO{Title: "No Kind"})
	if err == nil {
		t.Fatal("Create() error = nil, want invalid argument")
	}
	if !cberrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("Create() code = %v, want ErrInvalidArgument", cberrors.ParseCoder(err))
	}
}

func TestCreatePersonalityUsesPersonalityCommand(t *testing.T) {
	personalityStub := &personalityCommandStub{}
	svc := NewService(Dependencies{
		PersonalityCommand: personalityStub,
	})

	result, err := svc.Create(context.Background(), CreateModelDTO{
		Kind:  KindPersonality,
		Code:  "personality_create",
		Title: "Personality",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !personalityStub.createCalled {
		t.Fatal("personality command Create was not called")
	}
	if result.Kind != KindPersonality {
		t.Fatalf("result kind = %s, want %s", result.Kind, KindPersonality)
	}
}

func TestGetQRCodeDispatchesByKind(t *testing.T) {
	personalityStub := &personalityCommandStub{}
	svc := NewService(Dependencies{
		PersonalityCommand: personalityStub,
		RawQRCodeGenerator: &personalityQRCodeGeneratorStub{},
	})

	t.Run("personality model", func(t *testing.T) {
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
		PersonalityCommand: &personalityCommandStub{},
	})

	got, err := svc.GetQRCode(context.Background(), "personality_demo")
	if err != nil {
		t.Fatalf("GetQRCode() error = %v", err)
	}
	if got != "/personality/assessment/personality_demo" {
		t.Fatalf("GetQRCode() = %q, want entry url fallback", got)
	}
}
