package plan

import (
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ==================== ID 类型定义 ====================

// AssessmentPlanID 测评计划ID类型
type AssessmentPlanID = meta.ID

// NewAssessmentPlanID 创建测评计划ID
func NewAssessmentPlanID() AssessmentPlanID {
	return meta.New()
}

// ParseAssessmentPlanID 解析测评计划ID
func ParseAssessmentPlanID(s string) (AssessmentPlanID, error) {
	return meta.ParseID(s)
}

// AssessmentTaskID 测评任务ID类型
type AssessmentTaskID = meta.ID

// NewAssessmentTaskID 创建测评任务ID
func NewAssessmentTaskID() AssessmentTaskID {
	return meta.New()
}

// ParseAssessmentTaskID 解析测评任务ID
func ParseAssessmentTaskID(s string) (AssessmentTaskID, error) {
	return meta.ParseID(s)
}

// ==================== 计划状态枚举 ====================

// PlanStatus 计划状态
type PlanStatus string

const (
	// PlanStatusActive 活跃：计划正在执行中
	PlanStatusActive PlanStatus = "active"
	// PlanStatusPaused 暂停：计划暂时停止，可恢复
	PlanStatusPaused PlanStatus = "paused"
	// PlanStatusFinished 已完成：所有任务已完成或计划到期
	PlanStatusFinished PlanStatus = "finished"
	// PlanStatusCanceled 已取消：计划被取消，不可恢复
	PlanStatusCanceled PlanStatus = "canceled"
)

// String 返回状态的字符串表示
func (s PlanStatus) String() string {
	return string(s)
}

// IsValid 检查状态是否有效
func (s PlanStatus) IsValid() bool {
	switch s {
	case PlanStatusActive, PlanStatusPaused, PlanStatusFinished, PlanStatusCanceled:
		return true
	default:
		return false
	}
}

// IsActive 是否活跃状态
func (s PlanStatus) IsActive() bool {
	return s == PlanStatusActive
}

// IsPaused 是否暂停状态
func (s PlanStatus) IsPaused() bool {
	return s == PlanStatusPaused
}

// IsFinished 是否已完成状态
func (s PlanStatus) IsFinished() bool {
	return s == PlanStatusFinished
}

// IsCanceled 是否已取消状态
func (s PlanStatus) IsCanceled() bool {
	return s == PlanStatusCanceled
}

// IsTerminal 是否终态（不可再迁移）
func (s PlanStatus) IsTerminal() bool {
	return s == PlanStatusFinished || s == PlanStatusCanceled
}

// ==================== 计划周期类型枚举 ====================

// PlanScheduleType 计划周期类型
type PlanScheduleType string

const (
	// PlanScheduleByWeek 每 N 周一次
	PlanScheduleByWeek PlanScheduleType = "by_week"
	// PlanScheduleByDay 每 N 天一次
	PlanScheduleByDay PlanScheduleType = "by_day"
	// PlanScheduleFixedDate 指定日期列表
	PlanScheduleFixedDate PlanScheduleType = "fixed_date"
	// PlanScheduleCustom 自定义周次（如 2,4,8,12）
	PlanScheduleCustom PlanScheduleType = "custom"
)

// String 返回周期类型的字符串表示
func (t PlanScheduleType) String() string {
	return string(t)
}

// IsValid 检查周期类型是否有效
func (t PlanScheduleType) IsValid() bool {
	switch t {
	case PlanScheduleByWeek, PlanScheduleByDay, PlanScheduleFixedDate, PlanScheduleCustom:
		return true
	default:
		return false
	}
}

// ==================== 任务状态枚举 ====================

// TaskStatus 任务状态
type TaskStatus string

const (
	// TaskStatusPending 待推送：任务已创建，但尚未到计划时间
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusOpened 已推送：已生成入口，用户可填写
	TaskStatusOpened TaskStatus = "opened"
	// TaskStatusCompleted 已完成：用户已提交答卷
	TaskStatusCompleted TaskStatus = "completed"
	// TaskStatusExpired 已过期：超过截止时间未完成
	TaskStatusExpired TaskStatus = "expired"
	// TaskStatusCanceled 已取消：任务被取消
	TaskStatusCanceled TaskStatus = "canceled"
)

// String 返回状态的字符串表示
func (s TaskStatus) String() string {
	return string(s)
}

// IsValid 检查状态是否有效
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusPending, TaskStatusOpened, TaskStatusCompleted, TaskStatusExpired, TaskStatusCanceled:
		return true
	default:
		return false
	}
}

// IsPending 是否待推送状态
func (s TaskStatus) IsPending() bool {
	return s == TaskStatusPending
}

// IsOpened 是否已推送状态
func (s TaskStatus) IsOpened() bool {
	return s == TaskStatusOpened
}

// IsCompleted 是否已完成状态
func (s TaskStatus) IsCompleted() bool {
	return s == TaskStatusCompleted
}

// IsExpired 是否已过期状态
func (s TaskStatus) IsExpired() bool {
	return s == TaskStatusExpired
}

// IsCanceled 是否已取消状态
func (s TaskStatus) IsCanceled() bool {
	return s == TaskStatusCanceled
}

// IsTerminal 是否终态（不可再迁移）
func (s TaskStatus) IsTerminal() bool {
	return s == TaskStatusCompleted || s == TaskStatusExpired || s == TaskStatusCanceled
}
