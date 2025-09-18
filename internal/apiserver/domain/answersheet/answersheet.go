package answersheet

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answersheet/answer"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
	"github.com/yshujie/questionnaire-scale/pkg/log"
	v1 "github.com/yshujie/questionnaire-scale/pkg/meta/v1"
)

// AnswerSheet 答卷
type AnswerSheet struct {
	id                   v1.ID
	questionnaireCode    string
	questionnaireVersion string
	title                string
	score                float64
	answers              []answer.Answer
	writer               *user.Writer
	testee               *user.Testee
	createdAt            time.Time
	updatedAt            time.Time
}

// NewAnswerSheet 创建答卷
func NewAnswerSheet(questionnaireCode string, questionnaireVersion string, opts ...AnswerSheetOption) *AnswerSheet {
	a := &AnswerSheet{
		questionnaireCode:    questionnaireCode,
		questionnaireVersion: questionnaireVersion,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

type AnswerSheetOption func(*AnswerSheet)

func WithID(id v1.ID) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.id = id
	}
}

func WithQuestionnaireCode(questionnaireCode string) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.questionnaireCode = questionnaireCode
	}
}

func WithQuestionnaireVersion(questionnaireVersion string) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.questionnaireVersion = questionnaireVersion
	}
}

func WithTitle(title string) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.title = title
	}
}

func WithScore(score float64) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.score = score
	}
}

func WithAnswers(answers []answer.Answer) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.answers = answers
	}
}

func WithWriter(writer *user.Writer) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.writer = writer
	}
}

func WithTestee(testee *user.Testee) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.testee = testee
	}
}

func WithCreatedAt(createdAt time.Time) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.createdAt = createdAt
	}
}

func WithUpdatedAt(updatedAt time.Time) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.updatedAt = updatedAt
	}
}

func (a *AnswerSheet) SetID(id v1.ID) {
	a.id = id
}

func (a *AnswerSheet) GetID() v1.ID {
	return a.id
}

func (a *AnswerSheet) GetQuestionnaireCode() string {
	return a.questionnaireCode
}

func (a *AnswerSheet) GetQuestionnaireVersion() string {
	return a.questionnaireVersion
}

func (a *AnswerSheet) GetTitle() string {
	return a.title
}

func (a *AnswerSheet) GetScore() float64 {
	return a.score
}

func (a *AnswerSheet) GetWriter() *user.Writer {
	if a.writer == nil {
		log.Warnf("Writer is nil for answersheet")
		return nil
	}
	return a.writer
}

func (a *AnswerSheet) GetTestee() *user.Testee {
	if a.testee == nil {
		log.Warnf("Testee is nil for answersheet")
		return nil
	}
	return a.testee
}

func (a *AnswerSheet) GetCreatedAt() time.Time {
	return a.createdAt
}

func (a *AnswerSheet) GetUpdatedAt() time.Time {
	return a.updatedAt
}

func (a *AnswerSheet) GetAnswers() []answer.Answer {
	if a.answers == nil {
		return []answer.Answer{} // 返回空切片而不是 nil
	}
	return a.answers
}

func (a *AnswerSheet) GetAnswer(questionCode string) (answer.Answer, error) {
	for _, answer := range a.answers {
		if answer.GetQuestionCode() == questionCode {
			return answer, nil
		}
	}
	return answer.Answer{}, errors.WithCode(errCode.ErrAnswerNotFound, "answer not found")
}
