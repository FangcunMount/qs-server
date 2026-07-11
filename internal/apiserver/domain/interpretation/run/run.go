package run

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type ID = meta.ID

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusRunning, StatusSucceeded, StatusFailed:
		return true
	default:
		return false
	}
}

type FailureKind string

const (
	FailureKindInput    FailureKind = "input"
	FailureKindTemplate FailureKind = "template"
	FailureKindBuild    FailureKind = "build"
	FailureKindTimeout  FailureKind = "timeout"
	FailureKindInternal FailureKind = "internal"
)

// Failure stores only a safe, classifiable cause. Internal error chains stay
// in logs and tracing rather than a durable client-visible report record.
type Failure struct {
	Kind        FailureKind
	Code        string
	SafeMessage string
	Retryable   bool
}

func (f Failure) Validate() error {
	if f.Kind == "" {
		return fmt.Errorf("interpretation failure kind is required")
	}
	if f.Code == "" {
		return fmt.Errorf("interpretation failure code is required")
	}
	if f.SafeMessage == "" {
		return fmt.Errorf("interpretation failure safe message is required")
	}
	return nil
}

// InterpretationRun is one attempt under a ReportGeneration. It never owns
// report content and it cannot modify Evaluation facts.
type InterpretationRun struct {
	id           ID
	generationID meta.ID
	attempt      int
	status       Status
	failure      *Failure
	traceID      string
	startedAt    *time.Time
	finishedAt   *time.Time
}

func NewPending(id, generationID meta.ID, attempt int) (*InterpretationRun, error) {
	if id.IsZero() || generationID.IsZero() {
		return nil, fmt.Errorf("interpretation run id and generation id are required")
	}
	if attempt < 1 {
		return nil, fmt.Errorf("interpretation run attempt must be positive")
	}
	return &InterpretationRun{id: id, generationID: generationID, attempt: attempt, status: StatusPending}, nil
}

// Restore rehydrates a persisted execution attempt while validating its
// terminal facts. Persistence adapters cannot manufacture an invalid attempt.
func Restore(input RestoreInput) (*InterpretationRun, error) {
	if input.ID.IsZero() || input.GenerationID.IsZero() || input.Attempt < 1 {
		return nil, fmt.Errorf("interpretation run identity is invalid")
	}
	if !input.Status.IsValid() {
		return nil, fmt.Errorf("interpretation run status is invalid")
	}
	switch input.Status {
	case StatusPending:
		if input.StartedAt != nil || input.FinishedAt != nil || input.Failure != nil {
			return nil, fmt.Errorf("pending interpretation run has execution facts")
		}
	case StatusRunning:
		if input.StartedAt == nil || input.FinishedAt != nil || input.Failure != nil {
			return nil, fmt.Errorf("running interpretation run facts are invalid")
		}
	case StatusSucceeded:
		if input.StartedAt == nil || input.FinishedAt == nil || input.Failure != nil {
			return nil, fmt.Errorf("succeeded interpretation run facts are invalid")
		}
	case StatusFailed:
		if input.StartedAt == nil || input.FinishedAt == nil || input.Failure == nil {
			return nil, fmt.Errorf("failed interpretation run facts are required")
		}
		if err := input.Failure.Validate(); err != nil {
			return nil, err
		}
	}
	if input.StartedAt != nil && input.FinishedAt != nil && input.FinishedAt.Before(*input.StartedAt) {
		return nil, fmt.Errorf("interpretation run finished at precedes started at")
	}
	r := &InterpretationRun{
		id:           input.ID,
		generationID: input.GenerationID,
		attempt:      input.Attempt,
		status:       input.Status,
		traceID:      input.TraceID,
		startedAt:    copyTimePtr(input.StartedAt),
		finishedAt:   copyTimePtr(input.FinishedAt),
	}
	if input.Failure != nil {
		failure := *input.Failure
		r.failure = &failure
	}
	return r, nil
}

type RestoreInput struct {
	ID           ID
	GenerationID meta.ID
	Attempt      int
	Status       Status
	Failure      *Failure
	TraceID      string
	StartedAt    *time.Time
	FinishedAt   *time.Time
}

func Next(id meta.ID, latest *InterpretationRun) (*InterpretationRun, error) {
	if latest == nil {
		return nil, fmt.Errorf("latest interpretation run is required")
	}
	if latest.status != StatusFailed {
		return nil, fmt.Errorf("next interpretation run requires a failed latest run")
	}
	return NewPending(id, latest.generationID, latest.attempt+1)
}

func (r *InterpretationRun) Start(at time.Time, traceID string) error {
	if r == nil {
		return fmt.Errorf("interpretation run is required")
	}
	if r.status != StatusPending {
		return fmt.Errorf("interpretation run cannot start from status %s", r.status)
	}
	if at.IsZero() {
		return fmt.Errorf("interpretation run started at is required")
	}
	r.status = StatusRunning
	r.traceID = traceID
	r.startedAt = copyTime(at)
	return nil
}

func (r *InterpretationRun) Succeed(at time.Time) error {
	if r == nil {
		return fmt.Errorf("interpretation run is required")
	}
	if r.status != StatusRunning {
		return fmt.Errorf("interpretation run cannot succeed from status %s", r.status)
	}
	if at.IsZero() {
		return fmt.Errorf("interpretation run finished at is required")
	}
	r.status = StatusSucceeded
	r.failure = nil
	r.finishedAt = copyTime(at)
	return nil
}

func (r *InterpretationRun) Fail(at time.Time, failure Failure) error {
	if r == nil {
		return fmt.Errorf("interpretation run is required")
	}
	if r.status != StatusRunning {
		return fmt.Errorf("interpretation run cannot fail from status %s", r.status)
	}
	if at.IsZero() {
		return fmt.Errorf("interpretation run finished at is required")
	}
	if err := failure.Validate(); err != nil {
		return err
	}
	r.status = StatusFailed
	r.failure = &failure
	r.finishedAt = copyTime(at)
	return nil
}

func (r *InterpretationRun) ID() ID { return r.id }

func (r *InterpretationRun) GenerationID() meta.ID { return r.generationID }

func (r *InterpretationRun) Attempt() int { return r.attempt }

func (r *InterpretationRun) Status() Status { return r.status }

func (r *InterpretationRun) Failure() *Failure {
	if r.failure == nil {
		return nil
	}
	copy := *r.failure
	return &copy
}

func (r *InterpretationRun) TraceID() string { return r.traceID }

func (r *InterpretationRun) StartedAt() *time.Time { return copyTimePtr(r.startedAt) }

func (r *InterpretationRun) FinishedAt() *time.Time { return copyTimePtr(r.finishedAt) }

func copyTime(value time.Time) *time.Time { return &value }

func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
