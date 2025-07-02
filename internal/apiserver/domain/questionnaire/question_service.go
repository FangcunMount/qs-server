package questionnaire

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// QuestionService 问题服务
type QuestionService struct{}

// AddQuestion 添加问题
func (QuestionService) AddQuestion(q *Questionnaire, newQuestion question.Question) error {
	if newQuestion.GetCode().Value() == "" {
		return errors.WithCode(code.ErrQuestionnaireQuestionBasicInfoInvalid, "问题必须有 code")
	}
	for _, existing := range q.questions {
		if existing.GetCode() == newQuestion.GetCode() {
			return errors.WithCode(code.ErrQuestionnaireQuestionAlreadyExists, "code 重复，不能添加")
		}
	}
	q.questions = append(q.questions, newQuestion)
	return nil
}

// UpdateQuestion 更新问题
func (QuestionService) UpdateQuestion(q *Questionnaire, updated question.Question) error {
	for i := range q.questions {
		if q.questions[i].GetCode().Equals(updated.GetCode()) {
			q.questions[i] = updated
			return nil
		}
	}
	return errors.WithCode(code.ErrQuestionnaireQuestionNotFound, "找不到该题目")
}

// DeleteQuestion 删除问题
func (QuestionService) DeleteQuestion(q *Questionnaire, questionCode question.QuestionCode) error {
	for i := range q.questions {
		if q.questions[i].GetCode().Equals(questionCode) {
			q.questions = append(q.questions[:i], q.questions[i+1:]...)
			return nil
		}
	}

	return errors.WithCode(code.ErrQuestionnaireQuestionNotFound, "找不到该题目")
}
