package port

import (
	"context"
	"time"
)

// NotificationMeta 是 webhook/消息通道通用的通知元数据。
type NotificationMeta struct {
	EventID       string    `json:"event_id"`
	EventType     string    `json:"event_type"`
	AggregateType string    `json:"aggregate_type,omitempty"`
	AggregateID   string    `json:"aggregate_id,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// TaskCompletedNotification 是任务完成通知的标准载荷。
type TaskCompletedNotification struct {
	TaskID       string    `json:"task_id"`
	PlanID       string    `json:"plan_id"`
	TesteeID     string    `json:"testee_id"`
	AssessmentID string    `json:"assessment_id"`
	CompletedAt  time.Time `json:"completed_at"`
}

// TaskExpiredNotification 是任务过期通知的标准载荷。
type TaskExpiredNotification struct {
	TaskID    string    `json:"task_id"`
	PlanID    string    `json:"plan_id"`
	TesteeID  string    `json:"testee_id"`
	ExpiredAt time.Time `json:"expired_at"`
}

// TaskCanceledNotification 是任务取消通知的标准载荷。
type TaskCanceledNotification struct {
	TaskID     string    `json:"task_id"`
	PlanID     string    `json:"plan_id"`
	TesteeID   string    `json:"testee_id"`
	CanceledAt time.Time `json:"canceled_at"`
}

// TaskNotifier 定义 plan task 相关通知能力。
type TaskNotifier interface {
	NotifyTaskCompleted(ctx context.Context, meta NotificationMeta, payload TaskCompletedNotification) error
	NotifyTaskExpired(ctx context.Context, meta NotificationMeta, payload TaskExpiredNotification) error
	NotifyTaskCanceled(ctx context.Context, meta NotificationMeta, payload TaskCanceledNotification) error
}
