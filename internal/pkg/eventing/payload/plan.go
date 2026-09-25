package eventpayload

import "time"

// TaskOpenedData is the task opened event body.
type TaskOpenedData struct {
	TaskID   string    `json:"task_id"`
	PlanID   string    `json:"plan_id"`
	OrgID    int64     `json:"org_id"`
	TesteeID string    `json:"testee_id"`
	EntryURL string    `json:"entry_url"`
	OpenAt   time.Time `json:"open_at"`
}

// TaskOpenedReminderRequestedData is a durable reminder reference. The entry
// URL is read from the authoritative Task when handling the reminder, not
// copied into the Outbox message.
type TaskOpenedReminderRequestedData struct {
	TaskID           string    `json:"task_id"`
	PlanID           string    `json:"plan_id"`
	OrgID            int64     `json:"org_id"`
	TesteeID         string    `json:"testee_id"`
	OpenAt           time.Time `json:"open_at"`
	ScheduleRevision uint32    `json:"schedule_revision"`
}

// TaskCompletedData is the task completed event body.
type TaskCompletedData struct {
	TaskID       string    `json:"task_id"`
	PlanID       string    `json:"plan_id"`
	TesteeID     string    `json:"testee_id"`
	AssessmentID string    `json:"assessment_id"`
	CompletedAt  time.Time `json:"completed_at"`
}

// TaskExpiredData is the task expired event body.
type TaskExpiredData struct {
	TaskID    string    `json:"task_id"`
	PlanID    string    `json:"plan_id"`
	TesteeID  string    `json:"testee_id"`
	ExpiredAt time.Time `json:"expired_at"`
	Reason    string    `json:"reason,omitempty"`
}

// TaskCanceledData is the task canceled event body.
type TaskCanceledData struct {
	TaskID     string    `json:"task_id"`
	PlanID     string    `json:"plan_id"`
	TesteeID   string    `json:"testee_id"`
	CanceledAt time.Time `json:"canceled_at"`
}
