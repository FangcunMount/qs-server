package eventpayload

import "time"

// TaskOpenedData is the task opened event body.
type TaskOpenedData struct {
	TaskID   string    `json:"task_id"`
	PlanID   string    `json:"plan_id"`
	TesteeID string    `json:"testee_id"`
	EntryURL string    `json:"entry_url"`
	OpenAt   time.Time `json:"open_at"`
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
}

// TaskCanceledData is the task canceled event body.
type TaskCanceledData struct {
	TaskID     string    `json:"task_id"`
	PlanID     string    `json:"plan_id"`
	TesteeID   string    `json:"testee_id"`
	CanceledAt time.Time `json:"canceled_at"`
}
