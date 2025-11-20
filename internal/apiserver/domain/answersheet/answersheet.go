package answersheet

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/iam-contracts/pkg/log"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet/answer"
	errCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	// TODO: 重构为使用 actor.TesteeRef 和 actor.FillerRef
	// "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
)

// AnswerSheet 答卷
type AnswerSheet struct {
	id                   meta.ID
	questionnaireCode    string
	questionnaireVersion string
	title                string
	score                float64
	answers              []answer.Answer
	writer               interface{} // TODO: 重构为 actor.FillerRef
	testee               interface{} // TODO: 重构为 actor.TesteeRef
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

func WithID(id meta.ID) AnswerSheetOption {
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

// TODO: 待重构为使用 actor.FillerRef
func WithWriter(writer interface{}) AnswerSheetOption {
	return func(a *AnswerSheet) {
		a.writer = writer
	}
}

// TODO: 待重构为使用 actor.TesteeRef
func WithTestee(testee interface{}) AnswerSheetOption {
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

func (a *AnswerSheet) SetID(id meta.ID) {
	a.id = id
}

func (a *AnswerSheet) GetID() meta.ID {
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

func (a *AnswerSheet) GetWriter() interface{} {
	if a.writer == nil {
		log.Warnf("Writer is nil for answersheet")
		return nil
	}
	return a.writer
}

func (a *AnswerSheet) GetTestee() interface{} {
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
