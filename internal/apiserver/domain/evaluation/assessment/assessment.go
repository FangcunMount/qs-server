package assessment

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Assessment 测评聚合根
// 代表"一次具体的测评行为"，是 assessment 子域的核心聚合根
// 核心职责：
// 1. 记录测评元数据（谁做的、用什么问卷/解释模型、来源于哪个业务场景）
// 2. 管理测评生命周期（创建 → 提交 → evaluated/failed）
// 3. 记录评估结果摘要（总分、风险等级等兼容字段）
// 4. 发布 Evaluation 领域事件（AssessmentSubmittedEvent、AssessmentEvaluatedEvent、AssessmentFailedEvent）
type Assessment struct {
	// === 核心标识 ===
	id    ID
	orgID int64

	// === 关联实体引用（通过值对象引用，不直接持有对象）===
	testeeRef        testee.ID        // 受试者引用
	questionnaireRef QuestionnaireRef // 问卷引用
	answerSheetRef   AnswerSheetRef   // 答卷引用
	modelRef         *EvaluationModelRef

	// === 业务来源 ===
	origin Origin

	// === 状态与结果 ===
	status     Status
	totalScore *float64
	riskLevel  *RiskLevel
	summary    *ResultSummary

	// === 时间戳 ===
	submittedAt   *time.Time
	interpretedAt *time.Time
	failedAt      *time.Time

	// === 失败信息 ===
	failureReason *string

	// === 执行运行态（内存字段，暂不持久化） ===
	currentRunID evalrun.ID

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

// WithEvaluationModel 设置本次测评要使用的解释模型。
func WithEvaluationModel(modelRef EvaluationModelRef) AssessmentOption {
	return func(a *Assessment) {
		if modelRef.IsEmpty() {
			return
		}
		a.modelRef = &modelRef
	}
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

// ==================== 重建方法（仓储层使用）====================

// Reconstruct 从持久化数据重建测评对象
func Reconstruct(
	id ID,
	orgID int64,
	testeeID testee.ID,
	questionnaireRef QuestionnaireRef,
	answerSheetRef AnswerSheetRef,
	origin Origin,
	status Status,
	totalScore *float64,
	riskLevel *RiskLevel,
	submittedAt *time.Time,
	interpretedAt *time.Time,
	failedAt *time.Time,
	failureReason *string,
	modelRefs ...*EvaluationModelRef,
) *Assessment {
	var modelRef *EvaluationModelRef
	if len(modelRefs) > 0 {
		modelRef = modelRefs[0]
	}
	var summary *ResultSummary
	if totalScore != nil || riskLevel != nil {
		summary = summaryFromLegacyResult(totalScore, riskLevel)
	}
	return &Assessment{
		id:               id,
		orgID:            orgID,
		testeeRef:        testeeID,
		questionnaireRef: questionnaireRef,
		answerSheetRef:   answerSheetRef,
		modelRef:         modelRef,
		origin:           origin,
		status:           status,
		totalScore:       totalScore,
		riskLevel:        riskLevel,
		summary:          summary,
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
		a.orgID,
		a.id,
		a.testeeRef,
		a.questionnaireRef,
		a.answerSheetRef,
		a.modelRef,
		now,
	))

	return nil
}

// ApplyScoringOutcome 应用计分结果并将状态迁移为 evaluated。
func (a *Assessment) ApplyScoringOutcome(outcome *AssessmentOutcome) error {
	if !a.status.CanApplyScoring() {
		return NewInvalidStatusError("apply scoring", a.status)
	}
	if !a.HasEvaluationModel() {
		return ErrNoEvaluationModel
	}
	if outcome == nil {
		return ErrInvalidArgument
	}
	modelRef := outcome.ModelRef
	if modelRef.IsEmpty() {
		modelRef = *a.modelRef
		outcome.ModelRef = modelRef
	} else if !a.modelRef.SameIdentity(modelRef) {
		return ErrEvaluationModelMismatch
	}
	if outcome.Primary != nil {
		score := outcome.Primary.Value
		a.totalScore = &score
	}
	if outcome.Level != nil && IsRiskLevelCode(outcome.Level.Code) {
		risk := RiskLevel(outcome.Level.Code)
		a.riskLevel = &risk
	}
	summary := outcome.Summary
	a.summary = &summary
	a.status = StatusEvaluated
	return nil
}

// StageEvaluatedEvent records the durable outcome and run references that
// Interpretation must consume after scoring completes.
func (a *Assessment) StageEvaluatedEvent(evaluatedAt time.Time, outcomeID meta.ID, runID evalrun.ID) {
	a.addEvent(NewAssessmentEvaluatedEvent(
		a.orgID,
		a.id,
		a.testeeRef,
		outcomeID,
		runID,
		evaluatedAt,
	))
}

// MarkAsFailed 标记评估失败
// 前置条件：仅 submitted 状态可以标记 Evaluation 失败。
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
	a.interpretedAt = nil
	a.totalScore = nil
	a.riskLevel = nil
	a.summary = nil

	// 发布领域事件
	a.addEvent(NewAssessmentFailedEvent(
		a.orgID,
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
		a.orgID,
		a.id,
		a.testeeRef,
		a.questionnaireRef,
		a.answerSheetRef,
		a.modelRef,
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

// CurrentRunID 返回in-memory 活跃 评估执行 identifier。
func (a *Assessment) CurrentRunID() evalrun.ID {
	if a == nil {
		return ""
	}
	return a.currentRunID
}

// SetCurrentRunID 跟踪活跃 评估执行 in memory 仅。
func (a *Assessment) SetCurrentRunID(runID evalrun.ID) {
	if a == nil {
		return
	}
	a.currentRunID = runID
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

// EvaluationModelRef 获取解释模型引用。
func (a *Assessment) EvaluationModelRef() *EvaluationModelRef {
	return a.modelRef
}

// HasEvaluationModel 是否绑定了解释模型。
func (a *Assessment) HasEvaluationModel() bool {
	return a.modelRef != nil && !a.modelRef.IsEmpty()
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

// ResultSummary 获取通用结果摘要。
func (a *Assessment) ResultSummary() *ResultSummary {
	return a.summary
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

// NeedsEvaluation 是否需要评估（已提交且绑定了解释模型）
func (a *Assessment) NeedsEvaluation() bool {
	return a.IsSubmitted() && a.HasEvaluationModel()
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
	a.setID(NewID(id))
}

func summaryFromLegacyResult(totalScore *float64, riskLevel *RiskLevel) *ResultSummary {
	if totalScore == nil && riskLevel == nil {
		return nil
	}
	var score *float64
	if totalScore != nil {
		value := *totalScore
		score = &value
	}
	var level *string
	primary := ""
	if riskLevel != nil {
		value := string(*riskLevel)
		level = &value
		primary = value
	}
	return &ResultSummary{
		PrimaryLabel: primary,
		Score:        score,
		Level:        level,
	}
}
