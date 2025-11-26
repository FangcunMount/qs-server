package answersheet

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// AnswerSheet 答卷聚合根
// 答卷一旦创建就是已提交状态，不存在草稿（草稿在前端 localStorage）
// 答卷不可修改，是不可变对象
type AnswerSheet struct {
	id meta.ID

	// 填写者信息
	filler   *actor.FillerRef
	filledAt time.Time

	// 问卷&答卷信息
	questionnaireRef QuestionnaireRef
	answers          []Answer

	// 总分
	score float64
}

// NewAnswerSheet 创建答卷（提交时创建，不存在草稿状态）
func NewAnswerSheet(
	questionnaireRef QuestionnaireRef,
	filler *actor.FillerRef,
	answers []Answer,
	filledAt time.Time,
) (*AnswerSheet, error) {
	// 验证必填字段
	if questionnaireRef.IsEmpty() {
		return nil, fmt.Errorf("questionnaire reference is required")
	}
	if filler == nil {
		return nil, fmt.Errorf("filler is required")
	}
	if len(answers) == 0 {
		return nil, fmt.Errorf("at least one answer is required")
	}

	// 验证答案唯一性
	codeSet := make(map[string]bool)
	for _, ans := range answers {
		code := ans.QuestionCode()
		if codeSet[code] {
			return nil, fmt.Errorf("duplicate answer for question: %s", code)
		}
		codeSet[code] = true
	}

	return &AnswerSheet{
		questionnaireRef: questionnaireRef,
		filler:           filler,
		answers:          answers,
		filledAt:         filledAt,
		score:            0, // 初始分数为0，需要通过 CalculateScore 计算
	}, nil
}

// Reconstruct 从持久化数据重建答卷对象（用于仓储层）
func Reconstruct(
	id meta.ID,
	questionnaireRef QuestionnaireRef,
	filler *actor.FillerRef,
	answers []Answer,
	filledAt time.Time,
	score float64,
) *AnswerSheet {
	return &AnswerSheet{
		id:               id,
		questionnaireRef: questionnaireRef,
		filler:           filler,
		answers:          answers,
		filledAt:         filledAt,
		score:            score,
	}
}

// =========================
// 领域对象方法（充血模型）
// =========================

// ID 答卷标识（仅暴露必要的标识信息）
func (a *AnswerSheet) ID() meta.ID {
	return a.id
}

// AssignID 分配 ID（仅供仓储层使用）
func (a *AnswerSheet) AssignID(id meta.ID) {
	if a.id != 0 {
		panic("cannot reassign id to answer sheet")
	}
	a.id = id
}

// BelongsToQuestionnaire 判断是否属于指定问卷
func (a *AnswerSheet) BelongsToQuestionnaire(code, version string) bool {
	return a.questionnaireRef.Code() == code && a.questionnaireRef.Version() == version
}

// IsFilledBy 判断是否由指定填写者填写
func (a *AnswerSheet) IsFilledBy(filler *actor.FillerRef) bool {
	if a.filler == nil || filler == nil {
		return false
	}
	return a.filler.UserID() == filler.UserID()
}

// FindAnswer 查找指定问题的答案
func (a *AnswerSheet) FindAnswer(questionCode string) (Answer, bool) {
	for _, ans := range a.answers {
		if ans.QuestionCode() == questionCode {
			return ans, true
		}
	}
	return Answer{}, false
}

// AnswerCount 答案数量
func (a *AnswerSheet) AnswerCount() int {
	return len(a.answers)
}

// CalculateScore 计算总分（返回新的答卷对象，保持不可变性）
func (a *AnswerSheet) CalculateScore() *AnswerSheet {
	totalScore := 0.0
	for _, ans := range a.answers {
		totalScore += ans.Score()
	}

	newSheet := *a
	newSheet.score = totalScore
	return &newSheet
}

// Score 获取总分
func (a *AnswerSheet) Score() float64 {
	return a.score
}

// FilledAt 获取填写时间
func (a *AnswerSheet) FilledAt() time.Time {
	return a.filledAt
}

// Filler 获取填写者
func (a *AnswerSheet) Filler() *actor.FillerRef {
	return a.filler
}

// QuestionnaireInfo 获取问卷信息（用于展示）
func (a *AnswerSheet) QuestionnaireInfo() (code, version, title string) {
	return a.questionnaireRef.Code(), a.questionnaireRef.Version(), a.questionnaireRef.Title()
}

// Answers 获取所有答案的副本（防止外部修改）
func (a *AnswerSheet) Answers() []Answer {
	result := make([]Answer, len(a.answers))
	copy(result, a.answers)
	return result
}

// FilterAnswersByType 按问题类型筛选答案
func (a *AnswerSheet) FilterAnswersByType(questionType string) []Answer {
	filtered := make([]Answer, 0)
	for _, ans := range a.answers {
		if ans.QuestionType() == questionType {
			filtered = append(filtered, ans)
		}
	}
	return filtered
}

// ScoresByQuestion 获取每个问题的得分映射
func (a *AnswerSheet) ScoresByQuestion() map[string]float64 {
	scoreMap := make(map[string]float64)
	for _, ans := range a.answers {
		scoreMap[ans.QuestionCode()] = ans.Score()
	}
	return scoreMap
}
