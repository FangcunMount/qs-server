package plan

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// AssessmentTask 测评任务实体
// 代表计划分解后的"应测实例"，即"第 N 次测评"
//
// 核心职责：
// 1. 记录计划的第 N 次测评应该在什么时候进行
// 2. 管理任务状态（待推送、已推送、已完成、已过期）
// 3. 与 Assessment 关联（1:0..1 关系）
//
// 设计说明：
// - orgID 和 scaleCode 保留在 Task 中，用于查询优化和权限控制，避免 JOIN Plan 表
// - 审计字段（createdAt, updatedAt, version 等）由基础设施层处理，领域层不关心
type AssessmentTask struct {
	// === 核心标识 ===
	id     AssessmentTaskID
	planID AssessmentPlanID
	seq    int // 第 N 次测评

	// === 关联实体引用 ===
	orgID     int64 // 机构ID（用于查询优化和权限控制）
	testeeID  testee.ID
	scaleCode string // 量表编码（用于查询优化）

	// === 时间点 ===
	plannedAt   time.Time  // 计划时间点
	openAt      *time.Time // 实际开放时间
	expireAt    *time.Time // 截止时间
	completedAt *time.Time // 完成时间

	// === 状态与关联 ===
	status       TaskStatus
	assessmentID *assessment.ID

	// === 入口信息 ===
	entryToken string // 入口令牌（用于生成二维码/链接）
	entryURL   string // 入口 URL

	// === 领域事件 ===
	events []event.DomainEvent
}

// ==================== 构造函数 ====================

// NewAssessmentTask 创建测评任务
func NewAssessmentTask(
	planID AssessmentPlanID,
	seq int,
	orgID int64,
	testeeID testee.ID,
	scaleCode string,
	plannedAt time.Time,
) *AssessmentTask {
	return &AssessmentTask{
		id:        NewAssessmentTaskID(),
		planID:    planID,
		seq:       seq,
		orgID:     orgID,
		testeeID:  testeeID,
		scaleCode: scaleCode,
		plannedAt: plannedAt,
		status:    TaskStatusPending,
		events:    make([]event.DomainEvent, 0),
	}
}

// ==================== Getter 方法 ====================

// GetID 获取任务ID
func (t *AssessmentTask) GetID() AssessmentTaskID {
	return t.id
}

// GetPlanID 获取计划ID
func (t *AssessmentTask) GetPlanID() AssessmentPlanID {
	return t.planID
}

// GetSeq 获取序号
func (t *AssessmentTask) GetSeq() int {
	return t.seq
}

// GetOrgID 获取机构ID
func (t *AssessmentTask) GetOrgID() int64 {
	return t.orgID
}

// GetTesteeID 获取受试者ID
func (t *AssessmentTask) GetTesteeID() testee.ID {
	return t.testeeID
}

// GetScaleCode 获取量表编码
func (t *AssessmentTask) GetScaleCode() string {
	return t.scaleCode
}

// GetPlannedAt 获取计划时间点
func (t *AssessmentTask) GetPlannedAt() time.Time {
	return t.plannedAt
}

// GetOpenAt 获取开放时间
func (t *AssessmentTask) GetOpenAt() *time.Time {
	return t.openAt
}

// GetExpireAt 获取截止时间
func (t *AssessmentTask) GetExpireAt() *time.Time {
	return t.expireAt
}

// GetCompletedAt 获取完成时间
func (t *AssessmentTask) GetCompletedAt() *time.Time {
	return t.completedAt
}

// GetStatus 获取状态
func (t *AssessmentTask) GetStatus() TaskStatus {
	return t.status
}

// GetAssessmentID 获取关联的测评ID
func (t *AssessmentTask) GetAssessmentID() *assessment.ID {
	return t.assessmentID
}

// GetEntryToken 获取入口令牌
func (t *AssessmentTask) GetEntryToken() string {
	return t.entryToken
}

// GetEntryURL 获取入口URL
func (t *AssessmentTask) GetEntryURL() string {
	return t.entryURL
}

// IsPending 是否待推送状态
func (t *AssessmentTask) IsPending() bool {
	return t.status == TaskStatusPending
}

// IsOpened 是否已推送状态
func (t *AssessmentTask) IsOpened() bool {
	return t.status == TaskStatusOpened
}

// IsCompleted 是否已完成状态
func (t *AssessmentTask) IsCompleted() bool {
	return t.status == TaskStatusCompleted
}

// IsExpired 是否已过期状态
func (t *AssessmentTask) IsExpired() bool {
	return t.status == TaskStatusExpired
}

// IsCanceled 是否已取消状态
func (t *AssessmentTask) IsCanceled() bool {
	return t.status == TaskStatusCanceled
}

// IsTerminal 是否终态（不可再迁移）
func (t *AssessmentTask) IsTerminal() bool {
	return t.status.IsTerminal()
}

// ==================== 包内私有方法（供领域服务调用）===================

// open 开放任务（包内方法）
func (t *AssessmentTask) open(entryToken string, entryURL string, expireAt time.Time) error {
	if t.status != TaskStatusPending {
		return ErrTaskNotPending
	}

	now := time.Now()
	t.status = TaskStatusOpened
	t.openAt = &now
	t.expireAt = &expireAt
	t.entryToken = entryToken
	t.entryURL = entryURL

	// 发布任务开放事件
	t.addEvent(NewTaskOpenedEvent(
		t.id,
		t.planID,
		t.testeeID,
		t.entryURL,
		now,
	))

	return nil
}

// complete 完成任务（包内方法）
func (t *AssessmentTask) complete(assessmentID assessment.ID) error {
	if t.status != TaskStatusOpened {
		return ErrTaskNotOpened
	}

	now := time.Now()
	t.status = TaskStatusCompleted
	t.completedAt = &now
	t.assessmentID = &assessmentID

	// 发布任务完成事件
	t.addEvent(NewTaskCompletedEvent(
		t.id,
		t.planID,
		assessmentID,
		now,
	))

	return nil
}

// expire 过期任务（包内方法）
func (t *AssessmentTask) expire() error {
	if t.status != TaskStatusOpened {
		return ErrTaskNotOpened
	}

	now := time.Now()
	t.status = TaskStatusExpired

	// 发布任务过期事件
	t.addEvent(NewTaskExpiredEvent(
		t.id,
		t.planID,
		now,
	))

	return nil
}

// cancel 取消任务（包内方法）
func (t *AssessmentTask) cancel() {
	if t.status.IsTerminal() {
		return
	}

	t.status = TaskStatusCanceled
}

// ==================== 领域事件相关方法 ====================

// Events 获取待发布的领域事件
func (t *AssessmentTask) Events() []event.DomainEvent {
	return t.events
}

// ClearEvents 清空事件列表（通常在事件发布后调用）
func (t *AssessmentTask) ClearEvents() {
	t.events = make([]event.DomainEvent, 0)
}

// addEvent 添加领域事件（私有方法）
func (t *AssessmentTask) addEvent(evt event.DomainEvent) {
	if t.events == nil {
		t.events = make([]event.DomainEvent, 0)
	}
	t.events = append(t.events, evt)
}

// ==================== 仓储层辅助方法 ====================

// setID 设置ID（仅供仓储层使用）
func (t *AssessmentTask) setID(id AssessmentTaskID) {
	t.id = id
}

// RestoreFromRepository 从仓储恢复状态（仅供仓储层使用）
func (t *AssessmentTask) RestoreFromRepository(
	id AssessmentTaskID,
	status TaskStatus,
	openAt *time.Time,
	expireAt *time.Time,
	completedAt *time.Time,
	assessmentID *assessment.ID,
	entryToken string,
	entryURL string,
) {
	t.id = id
	t.status = status
	t.openAt = openAt
	t.expireAt = expireAt
	t.completedAt = completedAt
	t.assessmentID = assessmentID
	t.entryToken = entryToken
	t.entryURL = entryURL
}
