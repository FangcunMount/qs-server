package systemgovernance

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ReminderGapRecoveryRequest targets a missing batch for one original opening.
// It cannot change the event, recipients, deadline, or external-call receipts.
type ReminderGapRecoveryRequest struct {
	RequestID         string    `json:"request_id"`
	TaskID            string    `json:"task_id"`
	OpeningEventID    string    `json:"opening_event_id"`
	ExpectedVersion   uint64    `json:"expected_version"`
	OpenedBefore      time.Time `json:"opened_before"`
	EvidenceReference string    `json:"evidence_reference"`
	Reason            string    `json:"reason"`
	Confirm           bool      `json:"confirm"`
}

// ReminderGapRecovery is optional on the governance facade. Receipt lookup
// survives disabling authorization; missing receipts never permit a new ID.
type ReminderGapRecovery interface {
	AuthorizeReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ActionRunResult, error)
	ResolveReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ActionRunResult, bool, error)
}

func ValidateReminderGapRecovery(orgID int64, actorID uint64, req ReminderGapRecoveryRequest) error {
	id, err := strconv.ParseUint(req.TaskID, 10, 64)
	if orgID <= 0 || actorID == 0 || err != nil || id == 0 || id > math.MaxInt64 || strconv.FormatUint(id, 10) != req.TaskID ||
		req.ExpectedVersion == 0 || req.ExpectedVersion == math.MaxUint64 || !req.Confirm ||
		req.OpenedBefore.IsZero() || req.OpenedBefore.After(time.Now().Add(-10*time.Second)) {
		return errors.WithCode(code.ErrInvalidArgument, "confirmed original reminder identity and fixed past cutoff are required")
	}
	for _, field := range []struct {
		value string
		limit int
	}{{req.RequestID, 64}, {req.OpeningEventID, 64}, {req.EvidenceReference, 255}, {req.Reason, 1024}} {
		if strings.TrimSpace(field.value) == "" || strings.TrimSpace(field.value) != field.value || len(field.value) > field.limit {
			return errors.WithCode(code.ErrInvalidArgument, "bounded request identity, evidence and reason are required")
		}
	}
	return nil
}

func (f *facade) AuthorizeReminderGap(ctx context.Context, orgID int64, actorID uint64, req ReminderGapRecoveryRequest) (*ActionRunResult, error) {
	if f == nil || f.deps.ReminderGapRecovery == nil || !f.deps.ReminderGapRecoveryEnabled {
		return nil, errActionsUnavailable()
	}
	if err := ValidateReminderGapRecovery(orgID, actorID, req); err != nil {
		return nil, err
	}
	return f.deps.ReminderGapRecovery.AuthorizeReminderGap(ctx, orgID, actorID, req)
}

func (f *facade) ResolveReminderGap(ctx context.Context, orgID int64, actorID uint64, req ReminderGapRecoveryRequest) (*ActionRunResult, bool, error) {
	if f == nil || f.deps.ReminderGapRecovery == nil {
		return nil, false, errActionsUnavailable()
	}
	if err := ValidateReminderGapRecovery(orgID, actorID, req); err != nil {
		return nil, false, err
	}
	return f.deps.ReminderGapRecovery.ResolveReminderGap(ctx, orgID, actorID, req)
}
