package historicalseed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const Version1 = 1

var ErrOrgMismatch = errors.New("historical seed organization does not match request")

// Timeline contains business occurrence times. Runtime concerns such as auth,
// leases, retries and outbox scheduling must continue to use the system clock.
type Timeline struct {
	TesteeCreatedAt       *time.Time `json:"testee_created_at,omitempty"`
	EntryResolvedAt       *time.Time `json:"entry_resolved_at,omitempty"`
	EntryIntakeAt         *time.Time `json:"entry_intake_at,omitempty"`
	EnrollmentJoinedAt    *time.Time `json:"enrollment_joined_at,omitempty"`
	TaskOpenedAt          *time.Time `json:"task_opened_at,omitempty"`
	TaskCompletedAt       *time.Time `json:"task_completed_at,omitempty"`
	AnswerSheetFilledAt   *time.Time `json:"answersheet_filled_at,omitempty"`
	AssessmentCreatedAt   *time.Time `json:"assessment_created_at,omitempty"`
	AssessmentSubmittedAt *time.Time `json:"assessment_submitted_at,omitempty"`
	EvaluatedAt           *time.Time `json:"evaluated_at,omitempty"`
	ReportGeneratedAt     *time.Time `json:"report_generated_at,omitempty"`
}

// Context is the versioned cross-process contract for one historical scenario.
type Context struct {
	BatchID    string   `json:"batch_id"`
	ScenarioID string   `json:"scenario_id"`
	OrgID      uint64   `json:"org_id"`
	Version    int      `json:"version"`
	Timeline   Timeline `json:"timeline"`
}

func (c Context) Validate(earliest, latest time.Time, location *time.Location) error {
	if c.Version != Version1 {
		return fmt.Errorf("unsupported historical seed context version %d", c.Version)
	}
	if strings.TrimSpace(c.BatchID) == "" || strings.TrimSpace(c.ScenarioID) == "" {
		return errors.New("historical seed batch_id and scenario_id are required")
	}
	if c.OrgID == 0 {
		return errors.New("historical seed org_id is required")
	}
	if location == nil {
		location = time.UTC
	}

	ordered := []struct {
		name string
		at   *time.Time
	}{
		{"testee_created_at", c.Timeline.TesteeCreatedAt},
		{"entry_resolved_at", c.Timeline.EntryResolvedAt},
		{"entry_intake_at", c.Timeline.EntryIntakeAt},
		{"enrollment_joined_at", c.Timeline.EnrollmentJoinedAt},
		{"task_opened_at", c.Timeline.TaskOpenedAt},
		{"answersheet_filled_at", c.Timeline.AnswerSheetFilledAt},
		{"assessment_created_at", c.Timeline.AssessmentCreatedAt},
		{"assessment_submitted_at", c.Timeline.AssessmentSubmittedAt},
		{"task_completed_at", c.Timeline.TaskCompletedAt},
		{"evaluated_at", c.Timeline.EvaluatedAt},
		{"report_generated_at", c.Timeline.ReportGeneratedAt},
	}
	var previous *time.Time
	var previousName string
	for _, item := range ordered {
		if item.at == nil || item.at.IsZero() {
			continue
		}
		at := item.at.In(location)
		if !earliest.IsZero() && at.Before(startOfDay(earliest, location)) {
			return fmt.Errorf("historical seed %s is before allowed date", item.name)
		}
		if !latest.IsZero() && !at.Before(startOfDay(latest, location).AddDate(0, 0, 1)) {
			return fmt.Errorf("historical seed %s is after allowed date", item.name)
		}
		if previous != nil && at.Before(*previous) {
			return fmt.Errorf("historical seed timeline is out of order: %s is before %s", item.name, previousName)
		}
		copyAt := at
		previous = &copyAt
		previousName = item.name
	}
	return nil
}

func (c Context) ValidateOrg(orgID uint64) error {
	if orgID == 0 || c.OrgID != orgID {
		return ErrOrgMismatch
	}
	return nil
}

func startOfDay(value time.Time, location *time.Location) time.Time {
	value = value.In(location)
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, location)
}

type contextKey struct{}

func WithContext(ctx context.Context, historical Context) context.Context {
	return context.WithValue(ctx, contextKey{}, historical)
}

func FromContext(ctx context.Context) (Context, bool) {
	if ctx == nil {
		return Context{}, false
	}
	historical, ok := ctx.Value(contextKey{}).(Context)
	return historical, ok
}

// Clone returns an isolated copy suitable for event payloads and value objects.
func (c Context) Clone() Context {
	clone := c
	clone.Timeline.TesteeCreatedAt = cloneTime(c.Timeline.TesteeCreatedAt)
	clone.Timeline.EntryResolvedAt = cloneTime(c.Timeline.EntryResolvedAt)
	clone.Timeline.EntryIntakeAt = cloneTime(c.Timeline.EntryIntakeAt)
	clone.Timeline.EnrollmentJoinedAt = cloneTime(c.Timeline.EnrollmentJoinedAt)
	clone.Timeline.TaskOpenedAt = cloneTime(c.Timeline.TaskOpenedAt)
	clone.Timeline.TaskCompletedAt = cloneTime(c.Timeline.TaskCompletedAt)
	clone.Timeline.AnswerSheetFilledAt = cloneTime(c.Timeline.AnswerSheetFilledAt)
	clone.Timeline.AssessmentCreatedAt = cloneTime(c.Timeline.AssessmentCreatedAt)
	clone.Timeline.AssessmentSubmittedAt = cloneTime(c.Timeline.AssessmentSubmittedAt)
	clone.Timeline.EvaluatedAt = cloneTime(c.Timeline.EvaluatedAt)
	clone.Timeline.ReportGeneratedAt = cloneTime(c.Timeline.ReportGeneratedAt)
	return clone
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
