package definition

import (
	"context"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
)

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

func publishedQuestionnaireStub(code, version string, questions ...questionnaireapp.QuestionResult) questionnaireQueryStub {
	return questionnaireQueryStub{result: &questionnaireapp.QuestionnaireResult{
		Code: code, Version: version, Title: code, Status: "published", Questions: questions,
	}}
}
