package answersheet

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
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

	// 领域事件收集器
	events []event.DomainEvent
}

// NewAnswerSheet 创建答卷（提交时创建，不存在草稿状态）
// 创建即提交，自动触发 AnswerSheetSubmittedEvent
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

	sheet := &AnswerSheet{
		questionnaireRef: questionnaireRef,
		filler:           filler,
		answers:          answers,
		filledAt:         filledAt,
		score:            0, // 初始分数为0，需要通过 CalculateScore 计算
		events:           make([]event.DomainEvent, 0),
	}

	return sheet, nil
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

// IsFilledBy 判断是否由指定填写者填写
func (a *AnswerSheet) IsFilledBy(filler *actor.FillerRef) bool {
	if a.filler == nil || filler == nil {
		return false
	}
	return a.filler.UserID() == filler.UserID()
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

// ===================== 领域事件相关方法 =====================

// Events 获取待发布的领域事件
func (a *AnswerSheet) Events() []event.DomainEvent {
	return a.events
}

// ClearEvents 清空事件列表（通常在事件发布后调用）
func (a *AnswerSheet) ClearEvents() {
	a.events = make([]event.DomainEvent, 0)
}

// addEvent 添加领域事件（私有方法）
func (a *AnswerSheet) addEvent(evt event.DomainEvent) {
	if a.events == nil {
		a.events = make([]event.DomainEvent, 0)
	}
	a.events = append(a.events, evt)
}

// RaiseSubmittedEvent 在持久化后触发提交事件（确保带上持久化后的 ID）
// testeeID 和 orgID 由应用层传入，用于传递给测评层
func (a *AnswerSheet) RaiseSubmittedEvent(testeeID, orgID uint64) {
	a.addEvent(NewAnswerSheetSubmittedEvent(a, testeeID, orgID))
}
