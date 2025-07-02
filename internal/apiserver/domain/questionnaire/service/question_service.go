package service

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
)

// QuestionService 问题服务
type QuestionService struct{}

// AddQuestion 添加问题
func (QuestionService) AddQuestion(q *questionnaire.Questionnaire, newQuestion question.Question) error {
	if newQuestion.GetCode().Value() == "" {
		return errors.WithCode(code.ErrQuestionnaireQuestionBasicInfoInvalid, "问题必须有 code")
	}
	for _, existing := range q.GetQuestions() {
		if existing.GetCode() == newQuestion.GetCode() {
			return errors.WithCode(code.ErrQuestionnaireQuestionAlreadyExists, "code 重复，不能添加")
		}
	}
	q.SetQuestions(append(q.GetQuestions(), newQuestion))
	return nil
}

// UpdateQuestion 更新问题
func (QuestionService) UpdateQuestion(q *questionnaire.Questionnaire, updated question.Question) error {
	for i := range q.GetQuestions() {
		if q.GetQuestions()[i].GetCode().Equals(updated.GetCode()) {
			q.SetQuestions(append(q.GetQuestions()[:i], updated))
			return nil
		}
	}
	return errors.WithCode(code.ErrQuestionnaireQuestionNotFound, "找不到该题目")
}

// DeleteQuestion 删除问题
func (QuestionService) DeleteQuestion(q *questionnaire.Questionnaire, questionCode question.QuestionCode) error {
	for i := range q.GetQuestions() {
		if q.GetQuestions()[i].GetCode().Equals(questionCode) {
			q.SetQuestions(append(q.GetQuestions()[:i], q.GetQuestions()[i+1:]...))
			return nil
		}
	}

	return errors.WithCode(code.ErrQuestionnaireQuestionNotFound, "找不到该题目")
}
