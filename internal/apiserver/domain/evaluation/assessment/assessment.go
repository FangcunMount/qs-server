package assessment

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// Assessment 测评聚合根
// 代表"一次具体的测评行为"，是 assessment 子域的核心聚合根
// 核心职责：
// 1. 记录测评元数据（谁做的、用什么问卷/量表、来源于哪个业务场景）
// 2. 管理测评生命周期（创建 → 提交 → 评估 → 完成/失败）
// 3. 记录评估结果（总分、风险等级）
// 4. 发布领域事件（AssessmentSubmittedEvent、AssessmentInterpretedEvent）
type Assessment struct {
	// === 核心标识 ===
	id    ID
	orgID int64

	// === 关联实体引用（通过值对象引用，不直接持有对象）===
	testeeRef        testee.ID        // 受试者引用
	questionnaireRef QuestionnaireRef // 问卷引用
	answerSheetRef   AnswerSheetRef   // 答卷引用
	medicalScaleRef  *MedicalScaleRef // 量表引用（可选：纯问卷模式为 nil）

	// === 业务来源 ===
	origin Origin

	// === 状态与结果 ===
	status     Status
	totalScore *float64
	riskLevel  *RiskLevel

	// === 时间戳 ===
	submittedAt   *time.Time
	interpretedAt *time.Time
	failedAt      *time.Time

	// === 失败信息 ===
	failureReason *string

	// === 领域事件（未持久化，提交后清空）===
	events []DomainEvent
}

// ==================== 构造函数与工厂方法 ====================

// AssessmentOption 测评构造选项
type AssessmentOption func(*Assessment)

// NewAssessment 创建测评（工厂方法）
func NewAssessment(
	orgID int64,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	origin Origin,
	opts ...AssessmentOption,
) (*Assessment, error) {
	// 验证必填字段
	if orgID <= 0 {
		return nil, fmt.Errorf("orgID is required")
	}
	if testeeID.IsZero() {
		return nil, fmt.Errorf("testeeID is required")
	}
	if questionnaireRef.IsEmpty() {
		return nil, fmt.Errorf("questionnaire reference is required")
	}
	if answerSheetRef.IsEmpty() {
		return nil, fmt.Errorf("answer sheet reference is required")
	}
	if !origin.Type().IsValid() {
		return nil, fmt.Errorf("invalid origin type: %s", origin.Type())
	}

	a := &Assessment{
		orgID:            orgID,
		testeeRef:        testeeID,
		questionnaireRef: questionnaireRef,
		answerSheetRef:   answerSheetRef,
		origin:           origin,
		status:           StatusPending,
		events:           make([]DomainEvent, 0),
	}

	for _, opt := range opts {
		opt(a)
	}

	return a, nil
}

// WithMedicalScale 设置关联的量表
func WithMedicalScale(scaleRef MedicalScaleRef) AssessmentOption {
	return func(a *Assessment) { a.medicalScaleRef = &scaleRef }
}

// WithID 设置ID（用于重建）
func WithID(id ID) AssessmentOption {
	return func(a *Assessment) {
		a.id = id
	}
}

// WithStatus 设置状态（用于重建）
func WithStatus(status Status) AssessmentOption {
	return func(a *Assessment) {
		a.status = status
	}
}

// ==================== 快捷工厂方法 ====================

// NewAdhocAssessment 创建一次性测评
func NewAdhocAssessment(
	orgID int64,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	opts ...AssessmentOption,
) (*Assessment, error) {
	return NewAssessment(
		orgID,
		testeeID,
		questionnaireRef,
		answerSheetRef,
		NewAdhocOrigin(),
		opts...,
	)
}

// NewPlanAssessment 从测评计划创建测评
func NewPlanAssessment(
	orgID int64,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	planID string,
	opts ...AssessmentOption,
) (*Assessment, error) {
	return NewAssessment(
		orgID,
		testeeID,
		questionnaireRef,
		answerSheetRef,
		NewPlanOrigin(planID),
		opts...,
	)
}

// NewScreeningAssessment 从入校筛查创建测评
func NewScreeningAssessment(
	orgID int64,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	screeningProjectID string,
	opts ...AssessmentOption,
) (*Assessment, error) {
	return NewAssessment(
		orgID,
		testeeID,
		questionnaireRef,
		answerSheetRef,
		NewScreeningOrigin(screeningProjectID),
		opts...,
	)
}

// ==================== 重建方法（仓储层使用）====================

// Reconstruct 从持久化数据重建测评对象
func Reconstruct(
	id ID,
	orgID int64,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	medicalScaleRef *MedicalScaleRef,
	origin Origin,
	status Status,
	totalScore *float64,
	riskLevel *RiskLevel,
	submittedAt *time.Time,
	interpretedAt *time.Time,
	failedAt *time.Time,
	failureReason *string,
) *Assessment {
	return &Assessment{
		id:               id,
		orgID:            orgID,
		testeeRef:        testeeID,
		questionnaireRef: questionnaireRef,
		answerSheetRef:   answerSheetRef,
		medicalScaleRef:  medicalScaleRef,
		origin:           origin,
		status:           status,
		totalScore:       totalScore,
		riskLevel:        riskLevel,
		submittedAt:      submittedAt,
		interpretedAt:    interpretedAt,
		failedAt:         failedAt,
		failureReason:    failureReason,
		events:           make([]DomainEvent, 0),
	}
}

// ==================== 状态迁移方法 ====================

// Submit 提交答卷
// 前置条件：只有 pending 状态可以提交
// 后置条件：状态变为 submitted，发布 AssessmentSubmittedEvent
func (a *Assessment) Submit() error {
	if !a.status.IsPending() {
		return NewInvalidStatusError("submit", a.status)
	}

	now := time.Now()
	a.status = StatusSubmitted
	a.submittedAt = &now

	// 发布领域事件
	a.addEvent(NewAssessmentSubmittedEvent(
		a.id,
		a.testeeRef,
		a.questionnaireRef,
		a.answerSheetRef,
		a.medicalScaleRef,
		now,
	))

	return nil
}

// ApplyEvaluation 应用评估结果
// 前置条件：只有 submitted 状态可以应用评估结果，且必须绑定了量表
// 后置条件：状态变为 interpreted，记录评估结果，发布 AssessmentInterpretedEvent
func (a *Assessment) ApplyEvaluation(result *EvaluationResult) error {
	if !a.status.IsSubmitted() {
		return NewInvalidStatusError("apply evaluation", a.status)
	}
	if !a.HasMedicalScale() {
		return ErrNoScale
	}
	if result == nil {
		return ErrInvalidArgument
	}

	now := time.Now()
	a.totalScore = &result.TotalScore
	a.riskLevel = &result.RiskLevel
	a.status = StatusInterpreted
	a.interpretedAt = &now

	// 发布领域事件
	a.addEvent(NewAssessmentInterpretedEvent(
		a.id,
		a.testeeRef,
		*a.medicalScaleRef,
		result.TotalScore,
		result.RiskLevel,
		now,
	))

	return nil
}

// MarkAsFailed 标记评估失败
// 前置条件：只有 submitted 状态可以标记失败
// 后置条件：状态变为 failed，记录失败原因，发布 AssessmentFailedEvent
func (a *Assessment) MarkAsFailed(reason string) error {
	if !a.status.IsSubmitted() {
		return NewInvalidStatusError("mark as failed", a.status)
	}
	if reason == "" {
		return ErrInvalidArgument
	}

	now := time.Now()
	a.status = StatusFailed
	a.failedAt = &now
	a.failureReason = &reason

	// 发布领域事件
	a.addEvent(NewAssessmentFailedEvent(
		a.id,
		a.testeeRef,
		reason,
		now,
	))

	return nil
}

// RetryFromFailed 从失败状态重试
// 前置条件：只有 failed 状态可以重试
// 后置条件：状态变为 submitted，清除失败信息，发布 AssessmentSubmittedEvent
func (a *Assessment) RetryFromFailed() error {
	if !a.status.IsFailed() {
		return NewInvalidStatusError("retry from failed", a.status)
	}

	now := time.Now()
	a.status = StatusSubmitted
	a.submittedAt = &now
	a.failedAt = nil
	a.failureReason = nil

	// 发布领域事件（重新触发评估流程）
	a.addEvent(NewAssessmentSubmittedEvent(
		a.id,
		a.testeeRef,
		a.questionnaireRef,
		a.answerSheetRef,
		a.medicalScaleRef,
		now,
	))

	return nil
}

// ==================== 标识与基本信息查询方法 ====================

// ID 获取测评ID
func (a *Assessment) ID() ID {
	return a.id
}

// AssignID 分配ID（仅供仓储层使用）
func (a *Assessment) AssignID(id ID) {
	if !a.id.IsZero() {
		panic("cannot reassign id to assessment")
	}
	a.id = id
}

// OrgID 获取组织ID
func (a *Assessment) OrgID() int64 {
	return a.orgID
}

// ==================== 关联实体查询方法 ====================

// TesteeID 获取受试者ID
func (a *Assessment) TesteeID() testee.ID {
	return a.testeeRef
}

// QuestionnaireRef 获取问卷引用
func (a *Assessment) QuestionnaireRef() QuestionnaireRef {
	return a.questionnaireRef
}

// AnswerSheetRef 获取答卷引用
func (a *Assessment) AnswerSheetRef() AnswerSheetRef {
	return a.answerSheetRef
}

// MedicalScaleRef 获取量表引用
func (a *Assessment) MedicalScaleRef() *MedicalScaleRef {
	return a.medicalScaleRef
}

// HasMedicalScale 是否绑定了量表
func (a *Assessment) HasMedicalScale() bool {
	return a.medicalScaleRef != nil && !a.medicalScaleRef.IsEmpty()
}

// ==================== 业务来源查询方法 ====================

// Origin 获取业务来源
func (a *Assessment) Origin() Origin {
	return a.origin
}

// OriginType 获取来源类型
func (a *Assessment) OriginType() OriginType {
	return a.origin.Type()
}

// OriginID 获取来源ID
func (a *Assessment) OriginID() *string {
	return a.origin.ID()
}

// ==================== 状态与结果查询方法 ====================

// Status 获取状态
func (a *Assessment) Status() Status {
	return a.status
}

// TotalScore 获取总分
func (a *Assessment) TotalScore() *float64 {
	return a.totalScore
}

// RiskLevel 获取风险等级
func (a *Assessment) RiskLevel() *RiskLevel {
	return a.riskLevel
}

// ==================== 时间戳查询方法 ====================

// SubmittedAt 获取提交时间
func (a *Assessment) SubmittedAt() *time.Time {
	return a.submittedAt
}

// InterpretedAt 获取解读时间
func (a *Assessment) InterpretedAt() *time.Time {
	return a.interpretedAt
}

// FailedAt 获取失败时间
func (a *Assessment) FailedAt() *time.Time {
	return a.failedAt
}

// ==================== 失败信息查询方法 ====================

// FailureReason 获取失败原因
func (a *Assessment) FailureReason() *string {
	return a.failureReason
}

// ==================== 状态判断辅助方法 ====================

// IsPending 是否待提交状态
func (a *Assessment) IsPending() bool {
	return a.status.IsPending()
}

// IsSubmitted 是否已提交状态
func (a *Assessment) IsSubmitted() bool {
	return a.status.IsSubmitted()
}

// IsInterpreted 是否已解读状态
func (a *Assessment) IsInterpreted() bool {
	return a.status.IsInterpreted()
}

// IsFailed 是否失败状态
func (a *Assessment) IsFailed() bool {
	return a.status.IsFailed()
}

// IsCompleted 是否已完成（已解读或失败）
func (a *Assessment) IsCompleted() bool {
	return a.status.IsTerminal()
}

// NeedsEvaluation 是否需要评估（已提交且绑定了量表）
func (a *Assessment) NeedsEvaluation() bool {
	return a.IsSubmitted() && a.HasMedicalScale()
}

// ==================== 领域事件管理 ====================

// Events 获取待发布的领域事件
func (a *Assessment) Events() []DomainEvent {
	return a.events
}

// ClearEvents 清空领域事件（持久化后调用）
func (a *Assessment) ClearEvents() {
	a.events = make([]DomainEvent, 0)
}

// addEvent 添加领域事件（私有方法）
func (a *Assessment) addEvent(event DomainEvent) {
	a.events = append(a.events, event)
}

// setID 设置ID（仓储层使用，用于持久化后同步自增ID）
func (a *Assessment) setID(id ID) {
	a.id = id
}

// SyncIDFromRepository 从仓储层同步ID（供仓储层使用）
func SyncIDFromRepository(a *Assessment, id uint64) {
	a.setID(ID(id))
}
