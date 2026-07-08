package personalitysession

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

type fakeModelReader struct {
	model *typologymodel.PersonalityModelResponse
}

func (f *fakeModelReader) Get(context.Context, string) (*typologymodel.PersonalityModelResponse, error) {
	return f.model, nil
}

type fakeQuestionnaireReader struct {
	questionnaire *questionnaire.QuestionnaireResponse
}

func (f *fakeQuestionnaireReader) Get(context.Context, string, string) (*questionnaire.QuestionnaireResponse, error) {
	return f.questionnaire, nil
}

func TestServiceStartReturnsBoundQuestionnaire(t *testing.T) {
	svc := NewService(
		&fakeModelReader{model: &typologymodel.PersonalityModelResponse{
			Code:                 "MBTI_OEJTS",
			Version:              "1.0.0",
			Title:                "MBTI",
			QuestionnaireCode:    "MBTI_OEJTS",
			QuestionnaireVersion: "1.0.0",
			Status:               "published",
		}},
		&fakeQuestionnaireReader{questionnaire: &questionnaire.QuestionnaireResponse{
			Code:    "MBTI_OEJTS",
			Version: "1.0.0",
			Status:  "published",
		}},
	)

	resp, err := svc.Start(context.Background(), &StartSessionRequest{
		ModelCode: "MBTI_OEJTS",
		TesteeID:  7,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if resp.Questionnaire.Code != "MBTI_OEJTS" {
		t.Fatalf("questionnaire code = %q", resp.Questionnaire.Code)
	}
	if resp.SubmitContract.TesteeID != "7" {
		t.Fatalf("submit testee_id = %q, want 7", resp.SubmitContract.TesteeID)
	}
	if resp.Endpoints.Report == "" || resp.Endpoints.SubmitAnswerSheet == "" {
		t.Fatal("expected endpoint templates to be populated")
	}
}

func TestServiceStartReturnsNilForUnknownModel(t *testing.T) {
	svc := NewService(&fakeModelReader{}, &fakeQuestionnaireReader{})

	resp, err := svc.Start(context.Background(), &StartSessionRequest{
		ModelCode: "UNKNOWN",
		TesteeID:  7,
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if resp != nil {
		t.Fatalf("Start() = %#v, want nil", resp)
	}
}
