package questionnaire

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// QuestionManager 问题管理领域服务
// 负责问卷中问题的增删改操作
// 通过调用聚合根的私有方法来修改状态，保证领域完整性
type QuestionManager struct{}

// AddQuestion 添加问题到问卷
func (QuestionManager) AddQuestion(q *Questionnaire, question Question) error {
	return q.AddQuestion(question)
}

// RemoveQuestion 从问卷中移除指定问题
func (QuestionManager) RemoveQuestion(q *Questionnaire, questionCode meta.Code) error {
	return q.RemoveQuestion(questionCode)
}

// RemoveAllQuestions 清空问卷中的所有问题
func (QuestionManager) RemoveAllQuestions(q *Questionnaire) {
	q.RemoveAllQuestions()
}

// ReplaceQuestions 替换问卷的所有问题
// 先清空现有问题，再按顺序添加新问题
func (QuestionManager) ReplaceQuestions(q *Questionnaire, questions []Question) error {
	return q.ReplaceQuestions(questions)
}

// UpdateQuestion 更新指定问题
// 通过编码查找并替换问题
func (QuestionManager) UpdateQuestion(q *Questionnaire, updatedQuestion Question) error {
	return q.UpdateQuestion(updatedQuestion)
}

// ReorderQuestions 重新排序问题
// codes 参数按照新的顺序提供问题编码
func (QuestionManager) ReorderQuestions(q *Questionnaire, codes []meta.Code) error {
	return q.ReorderQuestions(codes)
}
