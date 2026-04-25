package plan

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ==================== 聚合根类型常量 ====================

const (
	// AggregateTypeTask 任务聚合根类型
	AggregateTypeTask = "AssessmentTask"
)

// ==================== 事件类型常量 ====================

const (
	// EventTypeTaskOpened 任务开放事件
	EventTypeTaskOpened = eventcatalog.TaskOpened
	// EventTypeTaskCompleted 任务完成事件
	EventTypeTaskCompleted = eventcatalog.TaskCompleted
	// EventTypeTaskExpired 任务过期事件
	EventTypeTaskExpired = eventcatalog.TaskExpired
	// EventTypeTaskCanceled 任务取消事件
	EventTypeTaskCanceled = eventcatalog.TaskCanceled
)

// ==================== 事件 Payload 定义 ====================

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

// TaskOpenedEvent 任务开放事件
type TaskOpenedEvent = event.Event[TaskOpenedData]

// TaskCompletedEvent 任务完成事件
type TaskCompletedEvent = event.Event[TaskCompletedData]

// TaskExpiredEvent 任务过期事件
type TaskExpiredEvent = event.Event[TaskExpiredData]

// TaskCanceledEvent 任务取消事件
type TaskCanceledEvent = event.Event[TaskCanceledData]

// ==================== 事件构造函数 ====================

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
