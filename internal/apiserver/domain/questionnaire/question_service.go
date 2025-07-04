package questionnaire

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/pkg/code"
	"github.com/yshujie/questionnaire-scale/pkg/errors"
	"github.com/yshujie/questionnaire-scale/pkg/log"
)

// QuestionService 问题服务
type QuestionService struct{}

// AddQuestion 添加问题
func (QuestionService) AddQuestion(q *Questionnaire, newQuestion question.Question) error {
	log.Infow("---- in QuestionService AddQuestion: ")

	// 检查问题对象是否为 nil
	if newQuestion == nil {
		log.Errorw("---- newQuestion is nil, skipping")
		return errors.WithCode(code.ErrQuestionnaireQuestionBasicInfoInvalid, "问题对象不能为空")
	}

	if newQuestion.GetCode().Value() == "" {
		return errors.WithCode(code.ErrQuestionnaireQuestionBasicInfoInvalid, "问题必须有 code")
	}
	for _, existing := range q.GetQuestions() {
		if existing.GetCode() == newQuestion.GetCode() {
			return errors.WithCode(code.ErrQuestionnaireQuestionAlreadyExists, "code 重复，不能添加")
		}
	}
	q.questions = append(q.questions, newQuestion)
	log.Infow("---- q.questions: ", "q.questions", q.questions)
	return nil
}

// UpdateQuestion 更新问题
func (QuestionService) UpdateQuestion(q *Questionnaire, updated question.Question) error {
	for i := range q.GetQuestions() {
		if q.GetQuestions()[i].GetCode().Equals(updated.GetCode()) {
			q.questions = append(q.questions[:i], updated)
			return nil
		}
	}
	return errors.WithCode(code.ErrQuestionnaireQuestionNotFound, "找不到该题目")
}

// DeleteQuestion 删除问题
func (QuestionService) DeleteQuestion(q *Questionnaire, questionCode question.QuestionCode) error {
	for i := range q.GetQuestions() {
		if q.GetQuestions()[i].GetCode().Equals(questionCode) {
			q.questions = append(q.questions[:i], q.questions[i+1:]...)
			return nil
		}
	}

	return errors.WithCode(code.ErrQuestionnaireQuestionNotFound, "找不到该题目")
}
