package answersheet

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type AnswerQuestionReader interface {
	FindByCodeVersion(context.Context, string, string) (*questionnaire.Questionnaire, error)
}
type AnswerQuestionReaderInjector interface{ SetAnswerQuestionReader(AnswerQuestionReader) }

func (s *managementService) SetAnswerQuestionReader(reader AnswerQuestionReader) {
	s.questions = reader
}

type AnswerQuestionResult struct {
	Code    string
	Type    string
	Stem    string
	Tips    string
	Options []AnswerOptionResult
}
type AnswerOptionResult struct {
	Code    string
	Content string
}

func (s *managementService) enrichAnswerQuestions(ctx context.Context, result *AnswerSheetResult) error {
	if s.questions == nil || result.QuestionnaireCode == "" || result.QuestionnaireVer == "" {
		return nil
	}
	q, err := s.questions.FindByCodeVersion(ctx, result.QuestionnaireCode, result.QuestionnaireVer)
	if questionnaire.IsNotFound(err) {
		return nil
	} // Historical raw answers remain readable; never substitute the current draft.
	if err != nil {
		return errors.WrapC(err, code.ErrDatabase, "读取答卷对应题版失败")
	}
	if q == nil {
		return nil
	}
	questions := make(map[string]questionnaire.Question)
	for _, item := range q.GetQuestions() {
		questions[item.GetCode().String()] = item
	}
	for i := range result.Answers {
		answer := &result.Answers[i]
		item := questions[answer.QuestionCode]
		if item == nil {
			continue
		}
		display := &AnswerQuestionResult{Code: item.GetCode().String(), Type: string(item.GetType()), Stem: item.GetStem(), Tips: item.GetTips(), Options: []AnswerOptionResult{}}
		for _, option := range item.GetOptions() {
			display.Options = append(display.Options, AnswerOptionResult{Code: option.GetCode().String(), Content: option.GetContent()})
		}
		answer.Question = display
	}
	return nil
}
