package questionnaire

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// QuestionManager 问题管理领域服务
// 负责问卷中问题的增删改操作
// 通过调用聚合根的私有方法来修改状态，保证领域完整性
type QuestionManager struct{}

// AddQuestion 添加问题到问卷
func (QuestionManager) AddQuestion(q *Questionnaire, question Question) error {
	if question == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问题对象不能为空")
	}

	// 调用聚合根的私有方法（会自动检查编码重复）
	return q.addQuestion(question)
}

// RemoveQuestion 从问卷中移除指定问题
func (QuestionManager) RemoveQuestion(q *Questionnaire, questionCode meta.Code) error {
	if questionCode.Value() == "" {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问题编码不能为空")
	}

	// 调用聚合根的私有方法
	return q.removeQuestion(questionCode)
}

// RemoveAllQuestions 清空问卷中的所有问题
func (QuestionManager) RemoveAllQuestions(q *Questionnaire) {
	// 直接访问私有字段 - 领域服务特权
	q.questions = []Question{}
}

// ReplaceQuestions 替换问卷的所有问题
// 先清空现有问题，再按顺序添加新问题
func (QuestionManager) ReplaceQuestions(q *Questionnaire, questions []Question) error {
	if len(questions) == 0 {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问题列表不能为空")
	}

	// 1. 验证所有问题的有效性和编码唯一性
	codes := make(map[string]bool)
	for i, question := range questions {
		if question == nil {
			return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个问题对象为空", i+1)
		}

		questionCode := question.GetCode().Value()
		if questionCode == "" {
			return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个问题的编码不能为空", i+1)
		}

		if codes[questionCode] {
			return errors.WithCode(errorCode.ErrQuestionAlreadyExists, "问题编码 %s 重复", questionCode)
		}
		codes[questionCode] = true
	}

	// 2. 清空现有问题
	q.questions = []Question{}

	// 3. 按顺序添加新问题（此时不会有重复，直接赋值）
	q.questions = append(q.questions, questions...)

	return nil
}

// UpdateQuestion 更新指定问题
// 通过编码查找并替换问题
func (QuestionManager) UpdateQuestion(q *Questionnaire, updatedQuestion Question) error {
	if updatedQuestion == nil {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问题对象不能为空")
	}

	targetCode := updatedQuestion.GetCode()
	if targetCode.Value() == "" {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "问题编码不能为空")
	}

	// 查找并替换
	for i, existingQuestion := range q.questions {
		if existingQuestion.GetCode() == targetCode {
			q.questions[i] = updatedQuestion
			return nil
		}
	}

	return errors.WithCode(errorCode.ErrQuestionnaireQuestionNotFound, "未找到编码为 %s 的问题", targetCode.Value())
}

// ReorderQuestions 重新排序问题
// codes 参数按照新的顺序提供问题编码
func (QuestionManager) ReorderQuestions(q *Questionnaire, codes []meta.Code) error {
	if len(codes) != q.QuestionCount() {
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidQuestion, "提供的编码数量与现有问题数量不匹配")
	}

	// 1. 构建编码到问题的映射
	questionMap := make(map[string]Question)
	for _, question := range q.questions {
		questionMap[question.GetCode().Value()] = question
	}

	// 2. 按照新顺序重建问题列表
	newQuestions := make([]Question, 0, len(codes))
	for i, codeItem := range codes {
		question, exists := questionMap[codeItem.Value()]
		if !exists {
			return errors.WithCode(errorCode.ErrQuestionnaireQuestionNotFound, "第 %d 个编码 %s 对应的问题不存在", i+1, codeItem.Value())
		}
		newQuestions = append(newQuestions, question)
	}

	// 3. 替换问题列表
	q.questions = newQuestions
	return nil
}
