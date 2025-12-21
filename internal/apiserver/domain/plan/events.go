package plan

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 聚合根类型常量 ====================

const (
	// AggregateTypePlan 计划聚合根类型
	AggregateTypePlan = "AssessmentPlan"
	// AggregateTypeTask 任务聚合根类型
	AggregateTypeTask = "AssessmentTask"
)

// ==================== 事件类型常量 ====================

const (
	// EventTypePlanCreated 计划创建事件
	EventTypePlanCreated = "plan.created"
	// EventTypeTaskOpened 任务开放事件
	EventTypeTaskOpened = "task.opened"
	// EventTypeTaskCompleted 任务完成事件
	EventTypeTaskCompleted = "task.completed"
	// EventTypeTaskExpired 任务过期事件
	EventTypeTaskExpired = "task.expired"
)

// ==================== 事件 Payload 定义 ====================

// PlanCreatedData 计划创建事件数据
type PlanCreatedData struct {
	PlanID    string    `json:"plan_id"`
	ScaleCode string    `json:"scale_code"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskOpenedData 任务开放事件数据
type TaskOpenedData struct {
	TaskID   string    `json:"task_id"`
	PlanID   string    `json:"plan_id"`
	TesteeID string    `json:"testee_id"`
	EntryURL string    `json:"entry_url"`
	OpenAt   time.Time `json:"open_at"`
}

// TaskCompletedData 任务完成事件数据
type TaskCompletedData struct {
	TaskID       string    `json:"task_id"`
	PlanID       string    `json:"plan_id"`
	AssessmentID string    `json:"assessment_id"`
	CompletedAt  time.Time `json:"completed_at"`
}

// TaskExpiredData 任务过期事件数据
type TaskExpiredData struct {
	TaskID    string    `json:"task_id"`
	PlanID    string    `json:"plan_id"`
	ExpiredAt time.Time `json:"expired_at"`
}

// ==================== 事件类型别名 ====================

// PlanCreatedEvent 计划创建事件
type PlanCreatedEvent = event.Event[PlanCreatedData]

// TaskOpenedEvent 任务开放事件
type TaskOpenedEvent = event.Event[TaskOpenedData]

// TaskCompletedEvent 任务完成事件
type TaskCompletedEvent = event.Event[TaskCompletedData]

// TaskExpiredEvent 任务过期事件
type TaskExpiredEvent = event.Event[TaskExpiredData]

// ==================== 事件构造函数 ====================

// NewPlanCreatedEvent 创建计划创建事件
func NewPlanCreatedEvent(
	planID AssessmentPlanID,
	scaleCode string,
	createdAt time.Time,
) PlanCreatedEvent {
	return event.New(
		EventTypePlanCreated,
		AggregateTypePlan,
		planID.String(),
		PlanCreatedData{
			PlanID:    planID.String(),
			ScaleCode: scaleCode,
			CreatedAt: createdAt,
		},
	)
}

// NewTaskOpenedEvent 创建任务开放事件
func NewTaskOpenedEvent(
	taskID AssessmentTaskID,
	planID AssessmentPlanID,
	testeeID testee.ID,
	entryURL string,
	openAt time.Time,
) TaskOpenedEvent {
	return event.New(
		EventTypeTaskOpened,
		AggregateTypeTask,
		taskID.String(),
		TaskOpenedData{
			TaskID:   taskID.String(),
			PlanID:   planID.String(),
			TesteeID: testeeID.String(),
			EntryURL: entryURL,
			OpenAt:   openAt,
		},
	)
}

// NewTaskCompletedEvent 创建任务完成事件
func NewTaskCompletedEvent(
	taskID AssessmentTaskID,
	planID AssessmentPlanID,
	assessmentID assessment.ID,
	completedAt time.Time,
) TaskCompletedEvent {
	return event.New(
		EventTypeTaskCompleted,
		AggregateTypeTask,
		taskID.String(),
		TaskCompletedData{
			TaskID:       taskID.String(),
			PlanID:       planID.String(),
			AssessmentID: assessmentID.String(),
			CompletedAt:  completedAt,
		},
	)
}

// NewTaskExpiredEvent 创建任务过期事件
func NewTaskExpiredEvent(
	taskID AssessmentTaskID,
	planID AssessmentPlanID,
	expiredAt time.Time,
) TaskExpiredEvent {
	return event.New(
		EventTypeTaskExpired,
		AggregateTypeTask,
		taskID.String(),
		TaskExpiredData{
			TaskID:    taskID.String(),
			PlanID:    planID.String(),
			ExpiredAt: expiredAt,
		},
	)
}
