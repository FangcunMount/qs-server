package answersheet

import (
	"fmt"
	"slices"
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
	submissionContext SubmissionContext
	filledAt          time.Time

	// 问卷&答卷信息
	questionnaireRef QuestionnaireRef
	answers          []Answer

	// 总分
	score float64

	// 领域事件收集器
	events []event.DomainEvent
}

// NewAnswerSheet 创建答卷（兼容旧调用方，不产生提交事件）。
//
// Deprecated: 新提交必须使用 Submit，让提交上下文和 SubmittedEvent 同时入模。
func NewAnswerSheet(
	questionnaireRef QuestionnaireRef,
	filler *actor.FillerRef,
	answers []Answer,
	filledAt time.Time,
) (*AnswerSheet, error) {
	if err := questionnaireRef.Validate(); err != nil {
		return nil, err
	}
	if filler == nil {
		return nil, fmt.Errorf("filler is required")
	}
	if err := validateAnswers(answers); err != nil {
		return nil, err
	}

	sheet := &AnswerSheet{
		questionnaireRef:  questionnaireRef,
		submissionContext: ReconstructSubmissionContext(filler, nil, meta.ZeroID, ""),
		answers:           answers,
		filledAt:          filledAt,
		score:             0, // 初始分数为0，需要通过 CalculateScore 计算
		events:            make([]event.DomainEvent, 0),
	}

	return sheet, nil
}

// Submit 创建完整的答卷提交事实，并立即产生 AnswerSheetSubmittedEvent。
func Submit(
	id meta.ID,
	questionnaireRef QuestionnaireRef,
	submissionContext SubmissionContext,
	answers []Answer,
	filledAt time.Time,
) (*AnswerSheet, error) {
	if id.IsZero() {
		return nil, fmt.Errorf("answer sheet id is required")
	}
	if err := questionnaireRef.Validate(); err != nil {
		return nil, err
	}
	if err := submissionContext.Validate(); err != nil {
		return nil, err
	}
	if err := validateAnswers(answers); err != nil {
		return nil, err
	}

	sheet := &AnswerSheet{
		id:                id,
		questionnaireRef:  questionnaireRef,
		submissionContext: submissionContext.clone(),
		answers:           answers,
		filledAt:          filledAt,
		score:             0,
		events:            make([]event.DomainEvent, 0, 1),
	}
	sheet.addEvent(NewAnswerSheetSubmittedEvent(sheet))
	return sheet, nil
}

func validateAnswers(answers []Answer) error {
	if len(answers) == 0 {
		return fmt.Errorf("at least one answer is required")
	}
	codeSet := make(map[string]bool)
	for _, ans := range answers {
		if err := ans.Validate(); err != nil {
			return err
		}
		code := ans.QuestionCode()
		if codeSet[code] {
			return fmt.Errorf("duplicate answer for question: %s", code)
		}
		codeSet[code] = true
	}
	return nil
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
	return ReconstructWithSubmissionContext(
		id,
		questionnaireRef,
		ReconstructSubmissionContext(filler, nil, meta.ZeroID, ""),
		answers,
		filledAt,
		score,
	)
}

// ReconstructWithSubmissionContext 从持久化数据重建带提交上下文的答卷对象。
func ReconstructWithSubmissionContext(
	id meta.ID,
	questionnaireRef QuestionnaireRef,
	submissionContext SubmissionContext,
	answers []Answer,
	filledAt time.Time,
	score float64,
) *AnswerSheet {
	return &AnswerSheet{
		id:                id,
		questionnaireRef:  questionnaireRef,
		submissionContext: submissionContext.clone(),
		answers:           answers,
		filledAt:          filledAt,
		score:             score,
	}
}

// ===  ===
// 领域对象方法（充血模型）
// ===  ===

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
	current := a.Filler()
	if current == nil || filler == nil {
		return false
	}
	return current.UserID() == filler.UserID()
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
	return a.submissionContext.Filler()
}

// SubmissionContext 获取提交上下文。
func (a *AnswerSheet) SubmissionContext() SubmissionContext {
	return a.submissionContext.clone()
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

// QuestionnaireRef 获取问卷引用
func (a *AnswerSheet) QuestionnaireRef() QuestionnaireRef {
	return a.questionnaireRef
}

// UpdateScores 更新答卷分数（领域方法）
// 根据计分结果更新每个答案的分数和总分
func (a *AnswerSheet) UpdateScores(scoredSheet *ScoredAnswerSheet) error {
	if scoredSheet == nil {
		return fmt.Errorf("scored answer sheet is required")
	}

	// 构建答案映射（按题目编码）
	answerMap := make(map[string]int) // question_code -> index 映射索引
	for i, ans := range a.answers {
		answerMap[ans.QuestionCode()] = i
	}

	// 更新每个答案的分数
	updatedAnswers := make([]Answer, len(a.answers))
	copy(updatedAnswers, a.answers)

	for _, scoredAns := range scoredSheet.ScoredAnswers {
		if idx, found := answerMap[scoredAns.QuestionCode]; found {
			// 使用不可变模式更新分数
			updatedAnswers[idx] = updatedAnswers[idx].WithScore(scoredAns.Score)
		}
	}

	// 更新答案列表和总分
	a.answers = updatedAnswers
	a.score = scoredSheet.TotalScore

	return nil
}

// ===================== 领域事件相关方法 =====================

// Events 获取待发布的领域事件
func (a *AnswerSheet) Events() []event.DomainEvent {
	return slices.Clone(a.events)
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
