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

// CurrentReader supports request-level replay before a stage mutates business
// facts. It resolves batch and scenario from the verified historical context.
type CurrentReader interface {
	FindCurrent(context.Context, string) (*Record, error)
}

type Reader interface {
	ListScenario(context.Context, uint64, string, string) ([]Record, error)
	ListBatch(context.Context, uint64, string, int, int) ([]Record, error)
}
