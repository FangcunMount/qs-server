package questionnaire

import (
	"context"
	"testing"
)

func TestQuestionnaireQRCodeQueryServiceUsesPublishedVersionWhenMissing(t *testing.T) {
	query := &questionnaireQRCodeQueryStub{
		published: &QuestionnaireResult{Code: "Q-1", Version: "v2"},
	}
	generator := &questionnaireQRCodeGeneratorStub{url: "https://example.test/q.png"}
	service := NewQRCodeQueryService(query, generator)

	got, err := service.GetQRCode(context.Background(), "Q-1", "")
	if err != nil {
		t.Fatalf("GetQRCode() error = %v", err)
	}
	if got != generator.url {
		t.Fatalf("GetQRCode() = %q, want %q", got, generator.url)
	}
	if !query.publishedCalled {
		t.Fatal("GetPublishedByCode was not called")
	}
	if generator.code != "Q-1" || generator.version != "v2" {
		t.Fatalf("generator called with code=%q version=%q, want Q-1/v2", generator.code, generator.version)
	}
}

func TestQuestionnaireQRCodeQueryServiceUsesExplicitVersion(t *testing.T) {
	query := &questionnaireQRCodeQueryStub{}
	generator := &questionnaireQRCodeGeneratorStub{url: "https://example.test/q.png"}
	service := NewQRCodeQueryService(query, generator)

	_, err := service.GetQRCode(context.Background(), "Q-1", "v1")
	if err != nil {
		t.Fatalf("GetQRCode() error = %v", err)
	}
	if query.publishedCalled {
		t.Fatal("GetPublishedByCode was called for explicit version")
	}
	if generator.version != "v1" {
		t.Fatalf("generator version = %q, want v1", generator.version)
	}
}

func TestQuestionnaireQRCodeQueryServiceRejectsMissingGenerator(t *testing.T) {
	service := NewQRCodeQueryService(&questionnaireQRCodeQueryStub{}, nil)

	if _, err := service.GetQRCode(context.Background(), "Q-1", "v1"); err == nil {
		t.Fatal("GetQRCode() error = nil, want error")
	}
}

type questionnaireQRCodeQueryStub struct {
	published       *QuestionnaireResult
	publishedCalled bool
}

func (s *questionnaireQRCodeQueryStub) GetByCode(context.Context, string) (*QuestionnaireResult, error) {
	return nil, nil
}

func (s *questionnaireQRCodeQueryStub) List(context.Context, ListQuestionnairesDTO) (*QuestionnaireSummaryListResult, error) {
	return nil, nil
}

func (s *questionnaireQRCodeQueryStub) GetPublishedByCode(context.Context, string) (*QuestionnaireResult, error) {
	s.publishedCalled = true
	return s.published, nil
}

func (s *questionnaireQRCodeQueryStub) GetQuestionCount(context.Context, string) (int32, error) {
	return 0, nil
}

func (s *questionnaireQRCodeQueryStub) ListPublished(context.Context, ListQuestionnairesDTO) (*QuestionnaireSummaryListResult, error) {
	return nil, nil
}

type questionnaireQRCodeGeneratorStub struct {
	url     string
	code    string
	version string
}

func (s *questionnaireQRCodeGeneratorStub) GenerateQuestionnaireQRCode(_ context.Context, code, version string) (string, error) {
	s.code = code
	s.version = version
	return s.url, nil
}
