package modelcatalog

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/cognitive"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/personality"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type behaviorCommandStub struct {
	createCalled bool
	qrCodeURL    string
	getErr       error
}

func (s *behaviorCommandStub) List(context.Context, behavior.ListInput) (*behavior.ListResult, error) {
	return nil, nil
}

func (s *behaviorCommandStub) Create(_ context.Context, input behavior.CreateInput) (*behavior.Model, error) {
	s.createCalled = true
	return &behavior.Model{Code: input.Code, Title: input.Title}, nil
}

func (s *behaviorCommandStub) Get(_ context.Context, modelCode string) (*behavior.Model, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	if modelCode == "behavior_demo" {
		return &behavior.Model{Code: modelCode}, nil
	}
	return nil, stderrors.New("not found")
}

func (s *behaviorCommandStub) UpdateBasicInfo(context.Context, behavior.UpdateBasicInfoInput) (*behavior.Model, error) {
	return nil, nil
}

func (s *behaviorCommandStub) Delete(context.Context, string) error { return nil }

func (s *behaviorCommandStub) Publish(context.Context, string) (*behavior.Model, error) {
	return nil, nil
}

func (s *behaviorCommandStub) Unpublish(context.Context, string) (*behavior.Model, error) {
	return nil, nil
}

func (s *behaviorCommandStub) Archive(context.Context, string) (*behavior.Model, error) {
	return nil, nil
}

func (s *behaviorCommandStub) BindQuestionnaire(_ context.Context, input behavior.BindQuestionnaireInput) (*behavior.Binding, error) {
	return &behavior.Binding{
		QuestionnaireCode:    input.QuestionnaireCode,
		QuestionnaireVersion: input.QuestionnaireVersion,
	}, nil
}

func (s *behaviorCommandStub) GetDefinition(context.Context, string) (*behavior.Definition, error) {
	return nil, nil
}

func (s *behaviorCommandStub) UpdateDefinition(context.Context, string, behavior.DefinitionInput) (*behavior.Definition, error) {
	return nil, nil
}

func (s *behaviorCommandStub) Options(context.Context) (*behavior.Options, error) {
	return nil, nil
}

func (s *behaviorCommandStub) GetQRCode(context.Context, string) (string, error) {
	return s.qrCodeURL, nil
}

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
		BehaviorCommand:    &behaviorCommandStub{},
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

func TestCreatePersonalityDoesNotDefaultToBehaviorAbility(t *testing.T) {
	personalityStub := &personalityCommandStub{}
	behaviorStub := &behaviorCommandStub{}
	svc := NewService(Dependencies{
		BehaviorCommand:    behaviorStub,
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
	if behaviorStub.createCalled {
		t.Fatal("behavior command Create should not be called")
	}
	if result.Kind != KindPersonality {
		t.Fatalf("result kind = %s, want %s", result.Kind, KindPersonality)
	}
}

func TestGetQRCodeDispatchesByKind(t *testing.T) {
	behaviorStub := &behaviorCommandStub{qrCodeURL: "https://example.com/scale.png"}
	personalityStub := &personalityCommandStub{}
	svc := NewService(Dependencies{
		BehaviorCommand:    behaviorStub,
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

	t.Run("behavior model", func(t *testing.T) {
		got, err := svc.GetQRCode(context.Background(), "behavior_demo")
		if err != nil {
			t.Fatalf("GetQRCode() error = %v", err)
		}
		if got != behaviorStub.qrCodeURL {
			t.Fatalf("GetQRCode() = %q, want behavior qrcode url", got)
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
