package plan

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// AssessmentPlan 测评计划聚合根
// 代表"周期性测评策略模板"，不关联具体的受试者
//
// 核心职责：
// 1. 描述"应该如何测"的策略（测什么、什么时候测）
// 2. 管理计划生命周期（启用、暂停、结束、取消）
// 3. 不关心每一次是否完成（由 Task 负责）
//
// 设计说明：
// - Plan 是模板，定义周期策略（如 SNAP 18 周测评）
// - 多个受试者可以加入同一个 Plan，每个受试者生成自己的 Task
// - Task 中关联 testeeID，通过 Task 可以查询某个受试者的计划
type AssessmentPlan struct {
	// === 核心标识 ===
	id    AssessmentPlanID
	orgID int64

	// === 关联实体引用 ===
	scaleID meta.ID

	// === 周期策略 ===
	// 所有周期策略都是相对时间窗口，不是绝对日期
	// 任务生成时：plannedAt = startDate（参数）+ 相对时间偏移
	scheduleType  PlanScheduleType
	interval      int         // 间隔 N 周/天（用于 by_week / by_day）
	totalTimes    int         // 计划总次数
	fixedDates    []time.Time // 固定日期列表（用于 fixed_date，特殊场景使用绝对日期）
	relativeWeeks []int       // 相对周次列表（用于 custom，如 [2,4,8,12,18]）

	// === 状态 ===
	status PlanStatus

	// === 领域事件 ===
	events []event.DomainEvent
}

// ==================== 构造函数 ====================

// NewAssessmentPlan 创建测评计划模板
// 注意：
// - startDate 不在计划中，而是任务生成时的参数
// - testeeID 不在计划中，计划是模板，多个受试者可以加入
// - 审计字段（createdAt, updatedAt, version 等）由基础设施层处理，领域层不关心
func NewAssessmentPlan(
	orgID int64,
	scaleID meta.ID,
	scheduleType PlanScheduleType,
	interval int,
	totalTimes int,
	opts ...PlanOption,
) (*AssessmentPlan, error) {
	// 验证必填字段
	if orgID <= 0 {
		return nil, ErrInvalidInterval
	}
	if scaleID.IsZero() {
		return nil, ErrPlanNotFound
	}
	if !scheduleType.IsValid() {
		return nil, ErrInvalidScheduleType
	}
	if interval <= 0 && (scheduleType == PlanScheduleByWeek || scheduleType == PlanScheduleByDay) {
		return nil, ErrInvalidInterval
	}
	// totalTimes 的验证：只有 by_week 和 by_day 类型需要验证
	// custom 和 fixed_date 类型的 totalTimes 由 relative_weeks 或 fixed_dates 的长度决定
	if totalTimes <= 0 && (scheduleType == PlanScheduleByWeek || scheduleType == PlanScheduleByDay) {
		return nil, ErrInvalidTotalTimes
	}

	plan := &AssessmentPlan{
		id:           NewAssessmentPlanID(),
		orgID:        orgID,
		scaleID:      scaleID,
		scheduleType: scheduleType,
		interval:     interval,
		totalTimes:   totalTimes,
		status:       PlanStatusActive,
		events:       make([]event.DomainEvent, 0),
	}

	// 应用选项
	for _, opt := range opts {
		opt(plan)
	}

	// 发布计划创建事件
	plan.addEvent(NewPlanCreatedEvent(
		plan.id,
		plan.scaleID,
		time.Now(),
	))

	return plan, nil
}

// PlanOption 计划构造选项
type PlanOption func(*AssessmentPlan)

// WithFixedDates 设置固定日期列表
func WithFixedDates(dates []time.Time) PlanOption {
	return func(p *AssessmentPlan) {
		p.fixedDates = dates
	}
}

// WithRelativeWeeks 设置相对周次列表
// 例如：[2, 4, 8, 12, 18] 表示相对于 startDate 的第 2、4、8、12、18 周
func WithRelativeWeeks(weeks []int) PlanOption {
	return func(p *AssessmentPlan) {
		p.relativeWeeks = weeks
	}
}

// ==================== Getter 方法 ====================

// GetID 获取计划ID
func (p *AssessmentPlan) GetID() AssessmentPlanID {
	return p.id
}

// GetOrgID 获取机构ID
func (p *AssessmentPlan) GetOrgID() int64 {
	return p.orgID
}

// GetScaleID 获取量表ID
func (p *AssessmentPlan) GetScaleID() meta.ID {
	return p.scaleID
}

// GetScheduleType 获取周期类型
func (p *AssessmentPlan) GetScheduleType() PlanScheduleType {
	return p.scheduleType
}

// GetInterval 获取间隔
func (p *AssessmentPlan) GetInterval() int {
	return p.interval
}

// GetTotalTimes 获取总次数
func (p *AssessmentPlan) GetTotalTimes() int {
	return p.totalTimes
}

// GetFixedDates 获取固定日期列表（返回副本）
func (p *AssessmentPlan) GetFixedDates() []time.Time {
	if p.fixedDates == nil {
		return nil
	}
	dates := make([]time.Time, len(p.fixedDates))
	copy(dates, p.fixedDates)
	return dates
}

// GetRelativeWeeks 获取相对周次列表（返回副本）
// 这些周次是相对于 startDate 的偏移，不是绝对日期
func (p *AssessmentPlan) GetRelativeWeeks() []int {
	if p.relativeWeeks == nil {
		return nil
	}
	weeks := make([]int, len(p.relativeWeeks))
	copy(weeks, p.relativeWeeks)
	return weeks
}

// GetStatus 获取状态
func (p *AssessmentPlan) GetStatus() PlanStatus {
	return p.status
}

// IsActive 是否活跃状态
func (p *AssessmentPlan) IsActive() bool {
	return p.status == PlanStatusActive
}

// IsPaused 是否暂停状态
func (p *AssessmentPlan) IsPaused() bool {
	return p.status == PlanStatusPaused
}

// IsFinished 是否已完成状态
func (p *AssessmentPlan) IsFinished() bool {
	return p.status == PlanStatusFinished
}

// IsCanceled 是否已取消状态
func (p *AssessmentPlan) IsCanceled() bool {
	return p.status == PlanStatusCanceled
}

// ==================== 包内私有方法（供领域服务调用）===================

// pause 暂停计划（包内方法）
func (p *AssessmentPlan) pause() error {
	if p.status != PlanStatusActive {
		return ErrPlanNotActive
	}
	p.status = PlanStatusPaused
	return nil
}

// resume 恢复计划（包内方法）
func (p *AssessmentPlan) resume() error {
	if p.status != PlanStatusPaused {
		return ErrPlanNotPaused
	}
	p.status = PlanStatusActive
	return nil
}

// finish 完成计划（包内方法）
func (p *AssessmentPlan) finish() {
	if p.status.IsTerminal() {
		return
	}
	p.status = PlanStatusFinished
}

// cancel 取消计划（包内方法）
func (p *AssessmentPlan) cancel() {
	if p.status.IsTerminal() {
		return
	}
	p.status = PlanStatusCanceled
}

// ==================== 领域事件相关方法 ====================

// Events 获取待发布的领域事件
func (p *AssessmentPlan) Events() []event.DomainEvent {
	return p.events
}

// ClearEvents 清空事件列表（通常在事件发布后调用）
func (p *AssessmentPlan) ClearEvents() {
	p.events = make([]event.DomainEvent, 0)
}

// addEvent 添加领域事件（私有方法）
func (p *AssessmentPlan) addEvent(evt event.DomainEvent) {
	if p.events == nil {
		p.events = make([]event.DomainEvent, 0)
	}
	p.events = append(p.events, evt)
}

// ==================== 仓储层辅助方法 ====================

// setID 设置ID（仅供仓储层使用）
func (p *AssessmentPlan) setID(id AssessmentPlanID) {
	p.id = id
}

// RestoreFromRepository 从仓储恢复状态（仅供仓储层使用）
func (p *AssessmentPlan) RestoreFromRepository(
	id AssessmentPlanID,
	status PlanStatus,
) {
	p.id = id
	p.status = status
}
