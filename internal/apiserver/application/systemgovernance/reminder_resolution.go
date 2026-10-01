package systemgovernance

import (
	"context"
	"time"
)

// ReminderResolutionRequest records a human finding, never authorizes a send.
// Reviewed is distinct from platform Confirmed; unknown findings stay unknown.
type ReminderResolutionRequest struct {
	RequestID         string    `json:"request_id"`
	DeliveryID        uint64    `json:"delivery_id"`
	TaskID            string    `json:"task_id"`
	OpeningEventID    string    `json:"opening_event_id"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	Finding           string    `json:"finding"`
	EvidenceReference string    `json:"evidence_reference"`
	Reason            string    `json:"reason"`
	Confirm           bool      `json:"confirm"`
}

// ReminderResolver only records a manual finding; it has no send capability.
type ReminderResolver interface {
	ResolveReminder(context.Context, int64, uint64, ReminderResolutionRequest) (*ActionRunResult, error)
	LoadReminderResolution(context.Context, int64, uint64, string) (*ActionRunResult, error)
}

func (f *facade) ResolveReminder(ctx context.Context, orgID int64, actorID uint64, req ReminderResolutionRequest) (*ActionRunResult, error) {
	if f == nil || f.deps.ReminderResolver == nil {
		return nil, errActionsUnavailable()
	}
	return f.deps.ReminderResolver.ResolveReminder(ctx, orgID, actorID, req)
}
func (f *facade) GetReminderResolution(ctx context.Context, orgID int64, actorID uint64, requestID string) (*ActionRunResult, error) {
	if f == nil || f.deps.ReminderResolver == nil {
		return nil, errActionsUnavailable()
	}
	return f.deps.ReminderResolver.LoadReminderResolution(ctx, orgID, actorID, requestID)
}
