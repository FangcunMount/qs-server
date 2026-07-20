package binding_test

import (
	"context"
	"testing"

	appbinding "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/binding"
	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestPublishedQuestionnairePolicyRequiresPublishedQuestions(t *testing.T) {
	t.Parallel()

	policy := appbinding.PublishedQuestionnairePolicy{
		Kind: domain.KindBehavioralRating,
		Questionnaires: questionnaireQueryStub{result: &questionnaireapp.QuestionnaireResult{
			Code: "Q1", Version: "1", Questions: []questionnaireapp.QuestionResult{{Code: "q1"}},
		}},
	}
	model := &domain.AssessmentModel{Kind: domain.KindBehavioralRating, Code: "B1"}
	got, err := policy.Validate(context.Background(), model, domain.QuestionnaireBinding{QuestionnaireCode: "Q1"})
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.QuestionnaireCode != "Q1" || got.QuestionnaireVersion != "1" {
		t.Fatalf("binding = %#v", got)
	}

	empty := appbinding.PublishedQuestionnairePolicy{
		Kind:           domain.KindCognitive,
		Questionnaires: questionnaireQueryStub{result: &questionnaireapp.QuestionnaireResult{Code: "Q1", Version: "1"}},
	}
	if _, err := empty.Validate(context.Background(), &domain.AssessmentModel{Kind: domain.KindCognitive}, domain.QuestionnaireBinding{QuestionnaireCode: "Q1"}); err == nil {
		t.Fatal("expected empty questions rejection")
	}
}

func TestPoliciesRejectUnregisteredKind(t *testing.T) {
	t.Parallel()
	policies := appbinding.NewPolicies(appbinding.TypologyPolicy{})
	_, err := policies.Validate(context.Background(), &domain.AssessmentModel{Kind: domain.KindCognitive}, domain.QuestionnaireBinding{QuestionnaireCode: "Q"})
	if err == nil {
		t.Fatal("expected unregistered kind rejection")
	}
}

type questionnaireQueryStub struct {
	result *questionnaireapp.QuestionnaireResult
	err    error
}

func (s questionnaireQueryStub) GetByCode(context.Context, string) (*questionnaireapp.QuestionnaireResult, error) {
	return s.result, s.err
}
func (s questionnaireQueryStub) List(context.Context, questionnaireapp.ListQuestionnairesDTO) (*questionnaireapp.QuestionnaireSummaryListResult, error) {
	return nil, nil
}
func (s questionnaireQueryStub) GetPublishedByCode(context.Context, string) (*questionnaireapp.QuestionnaireResult, error) {
	return s.result, s.err
}
func (s questionnaireQueryStub) GetPublishedByCodeVersion(context.Context, string, string) (*questionnaireapp.QuestionnaireResult, error) {
	return s.result, s.err
}
func (s questionnaireQueryStub) GetQuestionCount(context.Context, string) (int32, error) {
	if s.result == nil {
		return 0, s.err
	}
	return int32(len(s.result.Questions)), s.err
}
func (s questionnaireQueryStub) ListPublished(context.Context, questionnaireapp.ListQuestionnairesDTO) (*questionnaireapp.QuestionnaireSummaryListResult, error) {
	return nil, nil
}
