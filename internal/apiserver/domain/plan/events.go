package plan

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
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
	EventTypePlanCreated = eventconfig.PlanCreated
	// EventTypePlanTesteeEnrolled 受试者加入计划事件
	EventTypePlanTesteeEnrolled = eventconfig.PlanTesteeEnrolled
	// EventTypePlanTesteeTerminated 受试者终止计划参与事件
	EventTypePlanTesteeTerminated = eventconfig.PlanTesteeTerminated
	// EventTypePlanPaused 计划暂停事件
	EventTypePlanPaused = eventconfig.PlanPaused
	// EventTypePlanResumed 计划恢复事件
	EventTypePlanResumed = eventconfig.PlanResumed
	// EventTypePlanCanceled 计划取消事件
	EventTypePlanCanceled = eventconfig.PlanCanceled
	// EventTypePlanFinished 计划完成事件
	EventTypePlanFinished = eventconfig.PlanFinished
	// EventTypeTaskOpened 任务开放事件
	EventTypeTaskOpened = eventconfig.TaskOpened
	// EventTypeTaskCompleted 任务完成事件
	EventTypeTaskCompleted = eventconfig.TaskCompleted
	// EventTypeTaskExpired 任务过期事件
	EventTypeTaskExpired = eventconfig.TaskExpired
	// EventTypeTaskCanceled 任务取消事件
	EventTypeTaskCanceled = eventconfig.TaskCanceled
)

// ==================== 事件 Payload 定义 ====================

// PlanCreatedData 计划创建事件数据
type PlanCreatedData struct {
	PlanID    string    `json:"plan_id"`
	ScaleCode string    `json:"scale_code"`
	CreatedAt time.Time `json:"created_at"`
}

// PlanTesteeEnrolledData 受试者加入计划事件数据
type PlanTesteeEnrolledData struct {
	PlanID           string    `json:"plan_id"`
	TesteeID         string    `json:"testee_id"`
	OrgID            int64     `json:"org_id"`
	Idempotent       bool      `json:"idempotent"`
	CreatedTaskCount int       `json:"created_task_count"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// PlanTesteeTerminatedData 受试者终止计划参与事件数据
type PlanTesteeTerminatedData struct {
	PlanID            string    `json:"plan_id"`
	TesteeID          string    `json:"testee_id"`
	OrgID             int64     `json:"org_id"`
	AffectedTaskCount int       `json:"affected_task_count"`
	OccurredAt        time.Time `json:"occurred_at"`
}

// PlanPausedData 计划暂停事件数据
type PlanPausedData struct {
	PlanID    string    `json:"plan_id"`
	ScaleCode string    `json:"scale_code"`
	PausedAt  time.Time `json:"paused_at"`
}

// PlanResumedData 计划恢复事件数据
type PlanResumedData struct {
	PlanID    string    `json:"plan_id"`
	ScaleCode string    `json:"scale_code"`
	ResumedAt time.Time `json:"resumed_at"`
}

// PlanCanceledData 计划取消事件数据
type PlanCanceledData struct {
	PlanID     string    `json:"plan_id"`
	ScaleCode  string    `json:"scale_code"`
	CanceledAt time.Time `json:"canceled_at"`
}

// PlanFinishedData 计划完成事件数据
type PlanFinishedData struct {
	PlanID     string    `json:"plan_id"`
	ScaleCode  string    `json:"scale_code"`
	FinishedAt time.Time `json:"finished_at"`
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
	TesteeID     string    `json:"testee_id"`
	AssessmentID string    `json:"assessment_id"`
	CompletedAt  time.Time `json:"completed_at"`
}

// TaskExpiredData 任务过期事件数据
type TaskExpiredData struct {
	TaskID    string    `json:"task_id"`
	PlanID    string    `json:"plan_id"`
	TesteeID  string    `json:"testee_id"`
	ExpiredAt time.Time `json:"expired_at"`
}

// TaskCanceledData 任务取消事件数据
type TaskCanceledData struct {
	TaskID     string    `json:"task_id"`
	PlanID     string    `json:"plan_id"`
	TesteeID   string    `json:"testee_id"`
	CanceledAt time.Time `json:"canceled_at"`
}

// ==================== 事件类型别名 ====================

// PlanCreatedEvent 计划创建事件
type PlanCreatedEvent = event.Event[PlanCreatedData]

// PlanTesteeEnrolledEvent 受试者加入计划事件
type PlanTesteeEnrolledEvent = event.Event[PlanTesteeEnrolledData]

// PlanTesteeTerminatedEvent 受试者终止计划参与事件
type PlanTesteeTerminatedEvent = event.Event[PlanTesteeTerminatedData]

// PlanPausedEvent 计划暂停事件
type PlanPausedEvent = event.Event[PlanPausedData]

// PlanResumedEvent 计划恢复事件
type PlanResumedEvent = event.Event[PlanResumedData]

// PlanCanceledEvent 计划取消事件
type PlanCanceledEvent = event.Event[PlanCanceledData]

// PlanFinishedEvent 计划完成事件
type PlanFinishedEvent = event.Event[PlanFinishedData]

// TaskOpenedEvent 任务开放事件
type TaskOpenedEvent = event.Event[TaskOpenedData]

// TaskCompletedEvent 任务完成事件
type TaskCompletedEvent = event.Event[TaskCompletedData]

// TaskExpiredEvent 任务过期事件
type TaskExpiredEvent = event.Event[TaskExpiredData]

// TaskCanceledEvent 任务取消事件
type TaskCanceledEvent = event.Event[TaskCanceledData]

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

// NewPlanTesteeEnrolledEvent 创建受试者加入计划事件
func NewPlanTesteeEnrolledEvent(
	planID AssessmentPlanID,
	testeeID testee.ID,
	orgID int64,
	idempotent bool,
	createdTaskCount int,
	occurredAt time.Time,
) PlanTesteeEnrolledEvent {
	return event.New(
		EventTypePlanTesteeEnrolled,
		AggregateTypePlan,
		planID.String(),
		PlanTesteeEnrolledData{
			PlanID:           planID.String(),
			TesteeID:         testeeID.String(),
			OrgID:            orgID,
			Idempotent:       idempotent,
			CreatedTaskCount: createdTaskCount,
			OccurredAt:       occurredAt,
		},
	)
}

// NewPlanTesteeTerminatedEvent 创建受试者终止计划参与事件
func NewPlanTesteeTerminatedEvent(
	planID AssessmentPlanID,
	testeeID testee.ID,
	orgID int64,
	affectedTaskCount int,
	occurredAt time.Time,
) PlanTesteeTerminatedEvent {
	return event.New(
		EventTypePlanTesteeTerminated,
		AggregateTypePlan,
		planID.String(),
		PlanTesteeTerminatedData{
			PlanID:            planID.String(),
			TesteeID:          testeeID.String(),
			OrgID:             orgID,
			AffectedTaskCount: affectedTaskCount,
			OccurredAt:        occurredAt,
		},
	)
}

// NewPlanPausedEvent 创建计划暂停事件
func NewPlanPausedEvent(
	planID AssessmentPlanID,
	scaleCode string,
	pausedAt time.Time,
) PlanPausedEvent {
	return event.New(
		EventTypePlanPaused,
		AggregateTypePlan,
		planID.String(),
		PlanPausedData{
			PlanID:    planID.String(),
			ScaleCode: scaleCode,
			PausedAt:  pausedAt,
		},
	)
}

// NewPlanResumedEvent 创建计划恢复事件
func NewPlanResumedEvent(
	planID AssessmentPlanID,
	scaleCode string,
	resumedAt time.Time,
) PlanResumedEvent {
	return event.New(
		EventTypePlanResumed,
		AggregateTypePlan,
		planID.String(),
		PlanResumedData{
			PlanID:    planID.String(),
			ScaleCode: scaleCode,
			ResumedAt: resumedAt,
		},
	)
}

// NewPlanCanceledEvent 创建计划取消事件
func NewPlanCanceledEvent(
	planID AssessmentPlanID,
	scaleCode string,
	canceledAt time.Time,
) PlanCanceledEvent {
	return event.New(
		EventTypePlanCanceled,
		AggregateTypePlan,
		planID.String(),
		PlanCanceledData{
			PlanID:     planID.String(),
			ScaleCode:  scaleCode,
			CanceledAt: canceledAt,
		},
	)
}

// NewPlanFinishedEvent 创建计划完成事件
func NewPlanFinishedEvent(
	planID AssessmentPlanID,
	scaleCode string,
	finishedAt time.Time,
) PlanFinishedEvent {
	return event.New(
		EventTypePlanFinished,
		AggregateTypePlan,
		planID.String(),
		PlanFinishedData{
			PlanID:     planID.String(),
			ScaleCode:  scaleCode,
			FinishedAt: finishedAt,
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
	testeeID testee.ID,
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
			TesteeID:     testeeID.String(),
			AssessmentID: assessmentID.String(),
			CompletedAt:  completedAt,
		},
	)
}

// NewTaskExpiredEvent 创建任务过期事件
func NewTaskExpiredEvent(
	taskID AssessmentTaskID,
	planID AssessmentPlanID,
	testeeID testee.ID,
	expiredAt time.Time,
) TaskExpiredEvent {
	return event.New(
		EventTypeTaskExpired,
		AggregateTypeTask,
		taskID.String(),
		TaskExpiredData{
			TaskID:    taskID.String(),
			PlanID:    planID.String(),
			TesteeID:  testeeID.String(),
			ExpiredAt: expiredAt,
		},
	)
}

// NewTaskCanceledEvent 创建任务取消事件
func NewTaskCanceledEvent(
	taskID AssessmentTaskID,
	planID AssessmentPlanID,
	testeeID testee.ID,
	canceledAt time.Time,
) TaskCanceledEvent {
	return event.New(
		EventTypeTaskCanceled,
		AggregateTypeTask,
		taskID.String(),
		TaskCanceledData{
			TaskID:     taskID.String(),
			PlanID:     planID.String(),
			TesteeID:   testeeID.String(),
			CanceledAt: canceledAt,
		},
	)
}
