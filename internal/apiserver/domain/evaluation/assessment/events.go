package assessment

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/google/uuid"
)

// DomainEvent 领域事件接口
type DomainEvent interface {
	// EventID 事件唯一标识
	EventID() string

	// EventType 事件类型
	EventType() string

	// OccurredAt 事件发生时间
	OccurredAt() time.Time

	// AggregateID 聚合根ID
	AggregateID() ID
}

// ==================== 事件基类 ====================

// baseEvent 事件基类
type baseEvent struct {
	eventID    string
	eventType  string
	occurredAt time.Time
}

// newBaseEvent 创建事件基类
func newBaseEvent(eventType string) baseEvent {
	return baseEvent{
		eventID:    uuid.New().String(),
		eventType:  eventType,
		occurredAt: time.Now(),
	}
}

// EventID 获取事件ID
func (e baseEvent) EventID() string {
	return e.eventID
}

// EventType 获取事件类型
func (e baseEvent) EventType() string {
	return e.eventType
}

// OccurredAt 获取事件发生时间
func (e baseEvent) OccurredAt() time.Time {
	return e.occurredAt
}

// ==================== AssessmentSubmittedEvent ====================

// AssessmentSubmittedEvent 测评已提交事件
// 用途：
// - qs-worker 消费此事件，触发评估流程
// - 通知服务消费此事件，发送"答卷已提交"通知
type AssessmentSubmittedEvent struct {
	baseEvent

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
		baseEvent:        newBaseEvent("assessment.submitted"),
		assessmentID:     assessmentID,
		testeeID:         testeeID,
		questionnaireRef: questionnaireRef,
		answerSheetRef:   answerSheetRef,
		medicalScaleRef:  medicalScaleRef,
		submittedAt:      submittedAt,
	}
}

// AggregateID 获取聚合根ID
func (e *AssessmentSubmittedEvent) AggregateID() ID {
	return e.assessmentID
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
	baseEvent

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
		baseEvent:       newBaseEvent("assessment.interpreted"),
		assessmentID:    assessmentID,
		testeeID:        testeeID,
		medicalScaleRef: medicalScaleRef,
		totalScore:      totalScore,
		riskLevel:       riskLevel,
		interpretedAt:   interpretedAt,
	}
}

// AggregateID 获取聚合根ID
func (e *AssessmentInterpretedEvent) AggregateID() ID {
	return e.assessmentID
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
	baseEvent

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
		baseEvent:    newBaseEvent("assessment.failed"),
		assessmentID: assessmentID,
		testeeID:     testeeID,
		reason:       reason,
		failedAt:     failedAt,
	}
}

// AggregateID 获取聚合根ID
func (e *AssessmentFailedEvent) AggregateID() ID {
	return e.assessmentID
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
