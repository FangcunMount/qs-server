package historicalseedstage

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

const (
	StageEntryResolve        = "entry_resolve"
	StageEntryIntake         = "entry_intake"
	StagePlanEnrollment      = "plan_enrollment"
	StageTaskOpen            = "task_open"
	StageTaskComplete        = "task_complete"
	StageAnswerSheetSubmit   = "answersheet_submit"
	StageAssessmentCreated   = "assessment_created"
	StageAssessmentSubmitted = "assessment_submitted"
	StageOutcomeCommitted    = "outcome_committed"
	StageReportGenerated     = "report_generated"
)

var ErrPayloadConflict = errors.New("historical seed stage payload conflicts with completed result")

type Completion struct {
	Stage        string
	BusinessAt   time.Time
	ResourceType string
	ResourceID   string
	Payload      any
}

type Record struct {
	ID           uint64          `json:"id"`
	OrgID        uint64          `json:"org_id"`
	BatchID      string          `json:"batch_id"`
	ScenarioID   string          `json:"scenario_id"`
	Stage        string          `json:"stage"`
	PayloadHash  string          `json:"payload_hash"`
	Status       string          `json:"status"`
	BusinessAt   time.Time       `json:"business_at"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	PayloadJSON  json.RawMessage `json:"payload_json,omitempty"`
	ErrorText    string          `json:"error_text,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Recorder interface {
	Complete(context.Context, Completion) (*Record, error)
}

type Failure struct {
	Stage        string
	BusinessAt   time.Time
	ResourceType string
	ResourceID   string
	Payload      any
	Err          error
}

type Attempt struct {
	Stage        string
	BusinessAt   time.Time
	ResourceType string
	ResourceID   string
	Payload      any
}

// AttemptHandle identifies one diagnostic attempt exactly. It is never used as
// the business completion or idempotency authority.
type AttemptHandle struct {
	ID          uint64
	Stage       string
	ContextHash string
}

func (h AttemptHandle) IsZero() bool { return h.ID == 0 }

// AttemptLifecycle is diagnostic only. Completion records remain the sole
// idempotency and business-fact authority. CompleteAttempt must persist the
// immutable completion and close this exact handle in the caller transaction.
type AttemptLifecycle interface {
	Begin(context.Context, Attempt) (AttemptHandle, error)
	CompleteAttempt(context.Context, AttemptHandle, Completion) (*Record, error)
	Fail(context.Context, AttemptHandle, Failure) error
}

// CurrentReader supports request-level replay before a stage mutates business
// facts. It resolves batch and scenario from the verified historical context.
type CurrentReader interface {
	FindCurrent(context.Context, string) (*Record, error)
}

type Reader interface {
	ListScenario(context.Context, uint64, string, string) ([]Record, error)
	ListBatch(context.Context, uint64, string, int, int) ([]Record, error)
	ListScenarioAttempts(context.Context, uint64, string, string) ([]AttemptRecord, error)
	ListBatchAttempts(context.Context, uint64, string, int, int) ([]AttemptRecord, error)
}

type AttemptRecord struct {
	ID           uint64    `json:"id"`
	OrgID        uint64    `json:"org_id"`
	BatchID      string    `json:"batch_id"`
	ScenarioID   string    `json:"scenario_id"`
	Stage        string    `json:"stage"`
	AttemptNo    uint32    `json:"attempt_no"`
	ContextHash  string    `json:"context_hash"`
	Status       string    `json:"status"`
	BusinessAt   time.Time `json:"business_at"`
	ResourceType string    `json:"resource_type,omitempty"`
	ResourceID   string    `json:"resource_id,omitempty"`
	ErrorText    string    `json:"error_text,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type attemptContextKey struct{}

// BeginStageAttempt starts an attempt outside the business transaction and
// returns a context carrying the exact handle into the completion boundary.
func BeginStageAttempt(ctx context.Context, recorder Recorder, attempt Attempt) (context.Context, AttemptHandle, error) {
	lifecycle, ok := recorder.(AttemptLifecycle)
	if !ok || lifecycle == nil {
		return ctx, AttemptHandle{}, nil
	}
	handle, err := lifecycle.Begin(ctx, attempt)
	if err != nil || handle.IsZero() {
		return ctx, handle, err
	}
	handles, _ := ctx.Value(attemptContextKey{}).(map[string]AttemptHandle)
	copyHandles := make(map[string]AttemptHandle, len(handles)+1)
	for stage, value := range handles {
		copyHandles[stage] = value
	}
	copyHandles[handle.Stage] = handle
	return context.WithValue(ctx, attemptContextKey{}, copyHandles), handle, nil
}

// CompleteStage closes the exact handle carried for this stage. Legacy
// recorders retain their existing behavior.
func CompleteStage(ctx context.Context, recorder Recorder, completion Completion) (*Record, error) {
	if recorder == nil {
		return nil, nil
	}
	if lifecycle, ok := recorder.(AttemptLifecycle); ok {
		handles, _ := ctx.Value(attemptContextKey{}).(map[string]AttemptHandle)
		if handle, exists := handles[completion.Stage]; exists && !handle.IsZero() {
			return lifecycle.CompleteAttempt(ctx, handle, completion)
		}
	}
	return recorder.Complete(ctx, completion)
}

func FailStageAttempt(ctx context.Context, recorder Recorder, handle AttemptHandle, failure Failure) error {
	if handle.IsZero() {
		return nil
	}
	lifecycle, ok := recorder.(AttemptLifecycle)
	if !ok || lifecycle == nil {
		return nil
	}
	return lifecycle.Fail(ctx, handle, failure)
}
