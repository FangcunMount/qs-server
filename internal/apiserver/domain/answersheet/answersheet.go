package answersheet

import (
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/answer"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/user"
	errCode "github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// AnswerSheet 答卷
type AnswerSheet struct {
	ID                   uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	Title                string
	Score                uint16
	Answers              []answer.Answer
	Writer               *user.Writer
	Testee               *user.Testee
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// NewAnswerSheet 创建答卷
func NewAnswerSheet(questionnaireCode string, questionnaireVersion string, opts ...AnswerSheetOption) *AnswerSheet {
	a := &AnswerSheet{
		QuestionnaireCode:    questionnaireCode,
		QuestionnaireVersion: questionnaireVersion,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

type AnswerSheetOption func(*AnswerSheet)

func WithID(id uint64) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.ID = id
	}
}

func WithQuestionnaireCode(questionnaireCode string) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.QuestionnaireCode = questionnaireCode
	}
}

func WithQuestionnaireVersion(questionnaireVersion string) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.QuestionnaireVersion = questionnaireVersion
	}
}

func WithTitle(title string) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.Title = title
	}
}

func WithScore(score uint16) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.Score = score
	}
}

func WithAnswers(answers []answer.Answer) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.Answers = answers
	}
}

func WithWriter(writer *user.Writer) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.Writer = writer
	}
}

func WithTestee(testee *user.Testee) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.Testee = testee
	}
}

func WithCreatedAt(createdAt time.Time) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.CreatedAt = createdAt
	}
}

func WithUpdatedAt(updatedAt time.Time) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.UpdatedAt = updatedAt
	}
}

func (a *AnswerSheet) GetID() uint64 {
	return a.ID
}

func (a *AnswerSheet) GetQuestionnaireCode() string {
	return a.QuestionnaireCode
}

func (a *AnswerSheet) GetQuestionnaireVersion() string {
	return a.QuestionnaireVersion
}

func (a *AnswerSheet) GetTitle() string {
	return a.Title
}

func (a *AnswerSheet) GetScore() uint16 {
	return a.Score
}

func (a *AnswerSheet) GetWriter() *user.Writer {
	return a.Writer
}

func (a *AnswerSheet) GetTestee() *user.Testee {
	return a.Testee
}

func (a *AnswerSheet) GetCreatedAt() time.Time {
	return a.CreatedAt
}

func (a *AnswerSheet) GetUpdatedAt() time.Time {
	return a.UpdatedAt
}

func (a *AnswerSheet) GetAnswers() []answer.Answer {
	return a.Answers
}

func (a *AnswerSheet) GetAnswer(questionCode string) (answer.Answer, error) {
	for _, answer := range a.Answers {
		if answer.GetQuestionCode() == questionCode {
			return answer, nil
		}
	}
	return answer.Answer{}, errors.WithCode(errCode.ErrAnswerNotFound, "answer not found")
}
