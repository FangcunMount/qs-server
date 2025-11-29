package assessment

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// 重新导出共享内核的 DomainEvent 接口
// 这样 assessment 包的使用者无需直接依赖 shared/event 包
type DomainEvent = event.DomainEvent

// AggregateTypeName Assessment 聚合根类型名称
const AggregateTypeName = "Assessment"

// newBaseEvent 创建事件基类（内部辅助函数）
func newBaseEvent(eventType string, assessmentID ID) event.BaseEvent {
	return event.NewBaseEvent(eventType, AggregateTypeName, strconv.FormatInt(int64(assessmentID), 10))
}

// ==================== AssessmentSubmittedEvent ====================

// AssessmentSubmittedEvent 测评已提交事件
// 用途：
// - qs-worker 消费此事件，触发评估流程
// - 通知服务消费此事件，发送"答卷已提交"通知
type AssessmentSubmittedEvent struct {
	event.BaseEvent

	assessmentID     ID
	testeeID         testee.ID
	questionnaireRef QuestionnaireRef
	answerSheetRef   AnswerSheetRef
	medicalScaleRef  *MedicalScaleRef
	submittedAt      time.Time
}

// NewAssessmentSubmittedEvent 创建测评已提交事件
func NewAssessmentSubmittedEvent(
	assessmentID ID,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	medicalScaleRef *MedicalScaleRef,
	submittedAt time.Time,
) *AssessmentSubmittedEvent {
	return &AssessmentSubmittedEvent{
		BaseEvent:        newBaseEvent("assessment.submitted", assessmentID),
		assessmentID:     assessmentID,
		testeeID:         testeeID,
		questionnaireRef: questionnaireRef,
		answerSheetRef:   answerSheetRef,
		medicalScaleRef:  medicalScaleRef,
		submittedAt:      submittedAt,
	}
}

// AssessmentID 获取测评ID
func (e *AssessmentSubmittedEvent) AssessmentID() ID {
	return e.assessmentID
}

// TesteeID 获取受试者ID
func (e *AssessmentSubmittedEvent) TesteeID() testee.ID {
	return e.testeeID
}

// QuestionnaireRef 获取问卷引用
func (e *AssessmentSubmittedEvent) QuestionnaireRef() QuestionnaireRef {
	return e.questionnaireRef
}

// AnswerSheetRef 获取答卷引用
func (e *AssessmentSubmittedEvent) AnswerSheetRef() AnswerSheetRef {
	return e.answerSheetRef
}

// MedicalScaleRef 获取量表引用
func (e *AssessmentSubmittedEvent) MedicalScaleRef() *MedicalScaleRef {
	return e.medicalScaleRef
}

// SubmittedAt 获取提交时间
func (e *AssessmentSubmittedEvent) SubmittedAt() time.Time {
	return e.submittedAt
}

// NeedsEvaluation 是否需要评估（有量表才需要）
func (e *AssessmentSubmittedEvent) NeedsEvaluation() bool {
	return e.medicalScaleRef != nil && !e.medicalScaleRef.IsEmpty()
}

// ==================== AssessmentInterpretedEvent ====================

// AssessmentInterpretedEvent 测评已解读事件
// 用途：
// - 通知服务消费此事件，发送"报告已生成"通知
// - 预警服务消费此事件，对高风险案例发送预警
// - 统计服务消费此事件，更新实时统计数据
type AssessmentInterpretedEvent struct {
	event.BaseEvent

	assessmentID    ID
	testeeID        testee.ID
	medicalScaleRef MedicalScaleRef
	totalScore      float64
	riskLevel       RiskLevel
	interpretedAt   time.Time
}

// NewAssessmentInterpretedEvent 创建测评已解读事件
func NewAssessmentInterpretedEvent(
	assessmentID ID,
	testeeID testee.ID,
	medicalScaleRef MedicalScaleRef,
	totalScore float64,
	riskLevel RiskLevel,
	interpretedAt time.Time,
) *AssessmentInterpretedEvent {
	return &AssessmentInterpretedEvent{
		BaseEvent:       newBaseEvent("assessment.interpreted", assessmentID),
		assessmentID:    assessmentID,
		testeeID:        testeeID,
		medicalScaleRef: medicalScaleRef,
		totalScore:      totalScore,
		riskLevel:       riskLevel,
		interpretedAt:   interpretedAt,
	}
}

// AssessmentID 获取测评ID
func (e *AssessmentInterpretedEvent) AssessmentID() ID {
	return e.assessmentID
}

// TesteeID 获取受试者ID
func (e *AssessmentInterpretedEvent) TesteeID() testee.ID {
	return e.testeeID
}

// MedicalScaleRef 获取量表引用
func (e *AssessmentInterpretedEvent) MedicalScaleRef() MedicalScaleRef {
	return e.medicalScaleRef
}

// TotalScore 获取总分
func (e *AssessmentInterpretedEvent) TotalScore() float64 {
	return e.totalScore
}

// RiskLevel 获取风险等级
func (e *AssessmentInterpretedEvent) RiskLevel() RiskLevel {
	return e.riskLevel
}

// InterpretedAt 获取解读时间
func (e *AssessmentInterpretedEvent) InterpretedAt() time.Time {
	return e.interpretedAt
}

// IsHighRisk 是否高风险
func (e *AssessmentInterpretedEvent) IsHighRisk() bool {
	return IsHighRisk(e.riskLevel)
}

// ==================== AssessmentFailedEvent ====================

// AssessmentFailedEvent 测评失败事件
// 用途：
// - 日志服务记录失败原因
// - 监控服务统计失败率
// - 通知服务发送失败通知（可选）
type AssessmentFailedEvent struct {
	event.BaseEvent

	assessmentID ID
	testeeID     testee.ID
	reason       string
	failedAt     time.Time
}

// NewAssessmentFailedEvent 创建测评失败事件
func NewAssessmentFailedEvent(
	assessmentID ID,
	testeeID testee.ID,
	reason string,
	failedAt time.Time,
) *AssessmentFailedEvent {
	return &AssessmentFailedEvent{
		BaseEvent:    newBaseEvent("assessment.failed", assessmentID),
		assessmentID: assessmentID,
		testeeID:     testeeID,
		reason:       reason,
		failedAt:     failedAt,
	}
}

// AssessmentID 获取测评ID
func (e *AssessmentFailedEvent) AssessmentID() ID {
	return e.assessmentID
}

// TesteeID 获取受试者ID
func (e *AssessmentFailedEvent) TesteeID() testee.ID {
	return e.testeeID
}

// Reason 获取失败原因
func (e *AssessmentFailedEvent) Reason() string {
	return e.reason
}

// FailedAt 获取失败时间
func (e *AssessmentFailedEvent) FailedAt() time.Time {
	return e.failedAt
}
