package answersheet

import (
	"context"
	"fmt"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"testing"
)

type versionQuestionReader struct {
	code, version string
	q             *questionnaire.Questionnaire
	err           error
	calls         int
}

func (r *versionQuestionReader) FindByCodeVersion(_ context.Context, code, version string) (*questionnaire.Questionnaire, error) {
	r.code, r.version = code, version
	r.calls++
	return r.q, r.err
}

type displayQuestion struct{ questionnaire.Question }

func (displayQuestion) GetCode() meta.Code                  { return meta.NewCode("q1") }
func (displayQuestion) GetType() questionnaire.QuestionType { return questionnaire.TypeRadio }
func (displayQuestion) GetStem() string                     { return "Recorded question" }
func (displayQuestion) GetTips() string                     { return "Recorded tips" }
func (displayQuestion) GetOptions() []questionnaire.Option {
	o, _ := questionnaire.NewOptionWithStringCode("A", "Recorded option", 99)
	return []questionnaire.Option{o}
}

func TestAnswerDisplayUsesRecordedVersionAndOnlyAnsweredQuestions(t *testing.T) {
	q, err := questionnaire.NewQuestionnaire(meta.NewCode("Q1"), "Recorded", questionnaire.WithQuestions([]questionnaire.Question{displayQuestion{}}))
	if err != nil {
		t.Fatal(err)
	}
	reader := &versionQuestionReader{q: q}
	service := &managementService{questions: reader}
	result := &AnswerSheetResult{QuestionnaireCode: "Q1", QuestionnaireVer: "v1", Answers: []AnswerResult{{QuestionCode: "q1"}, {QuestionCode: "missing"}}}
	if err := service.enrichAnswerQuestions(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if reader.code != "Q1" || reader.version != "v1" {
		t.Fatal("did not read recorded version")
	}
	if result.Answers[0].Question == nil || result.Answers[0].Question.Options[0].Content != "Recorded option" {
		t.Fatalf("missing display: %+v", result.Answers)
	}
	if result.Answers[1].Question != nil {
		t.Fatal("invented historical question")
	}
}
func TestAnswerDisplayMissingHistoryNeverSubstitutesCurrentDraft(t *testing.T) {
	reader := &versionQuestionReader{}
	service := &managementService{questions: reader}
	result := &AnswerSheetResult{QuestionnaireCode: "Q1", QuestionnaireVer: "v1", Answers: []AnswerResult{{QuestionCode: "q1", Value: "A"}}}
	if err := service.enrichAnswerQuestions(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || result.Answers[0].Question != nil || result.Answers[0].Value != "A" {
		t.Fatal("historical answer changed")
	}
	reader.err = fmt.Errorf("storage unavailable")
	if err := service.enrichAnswerQuestions(context.Background(), result); err == nil {
		t.Fatal("storage error hidden")
	}
}
