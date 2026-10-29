package systemgovernance

import (
	"context"
	"encoding/hex"
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
	// Empty retains the historical protocol and its receipt input unchanged.
	SourceProtocol           string `json:"source_protocol,omitempty"`
	ExpectedScheduleRevision uint32 `json:"expected_schedule_revision,omitempty"`
	SourceFingerprint        string `json:"source_fingerprint,omitempty"`
}

const TransactionReminderSource = "transaction"

type ReminderGapInspection struct {
	TaskID            string    `json:"task_id"`
	OpeningEventID    string    `json:"opening_event_id"`
	ScheduleRevision  uint32    `json:"schedule_revision"`
	SourceFingerprint string    `json:"source_fingerprint"`
	Deadline          time.Time `json:"deadline"`
	Disposition       string    `json:"disposition"`
	ReadOnly          bool      `json:"read_only"`
}

// CurrentReminderGapRecovery uses original business effects and their ledgers;
// it never grants a new MQ execution or requeues a historical publishing row.
type CurrentReminderGapRecovery interface {
	ReminderGapRecovery
	InspectReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ReminderGapInspection, error)
	ResumeReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ActionRunResult, error)
}

// ReminderGapRecovery is optional on the governance facade. Receipt lookup
// survives disabling authorization; missing receipts never permit a new ID.
type ReminderGapRecovery interface {
	AuthorizeReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ActionRunResult, error)
	ResolveReminderGap(context.Context, int64, uint64, ReminderGapRecoveryRequest) (*ActionRunResult, bool, error)
}

func ValidateReminderGapRecovery(orgID int64, actorID uint64, req ReminderGapRecoveryRequest) error {
	if req.SourceProtocol == TransactionReminderSource {
		return ValidateTransactionReminderGap(orgID, actorID, req, false)
	}
	if req.SourceProtocol != "" || req.ExpectedScheduleRevision != 0 || req.SourceFingerprint != "" {
		return errors.WithCode(code.ErrInvalidArgument, "explicit reminder source protocol required")
	}
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

func ValidateTransactionReminderGap(orgID int64, actorID uint64, req ReminderGapRecoveryRequest, inspect bool) error {
	id, err := strconv.ParseUint(req.TaskID, 10, 64)
	if req.SourceProtocol != TransactionReminderSource || orgID <= 0 || actorID == 0 || err != nil || id == 0 || id > math.MaxInt64 || strconv.FormatUint(id, 10) != req.TaskID || req.ExpectedVersion != 0 || req.ExpectedScheduleRevision == 0 || req.OpenedBefore.IsZero() || req.OpenedBefore.After(time.Now().Add(-10*time.Second)) {
		return errors.WithCode(code.ErrInvalidArgument, "original transaction opening and fixed past cutoff required")
	}
	if inspect {
		return nil
	}
	raw, err := hex.DecodeString(req.SourceFingerprint)
	if err != nil || len(raw) != 32 || req.SourceFingerprint != strings.ToLower(req.SourceFingerprint) || !req.Confirm {
		return errors.WithCode(code.ErrInvalidArgument, "confirmed reviewed source fingerprint required")
	}
	for _, field := range []struct {
		value string
		limit int
	}{{req.RequestID, 64}, {req.OpeningEventID, 64}, {req.EvidenceReference, 255}, {req.Reason, 1024}} {
		if strings.TrimSpace(field.value) == "" || strings.TrimSpace(field.value) != field.value || len(field.value) > field.limit {
			return errors.WithCode(code.ErrInvalidArgument, "bounded original operation identity, evidence and reason required")
		}
	}
	return nil
}

func (f *facade) AuthorizeReminderGap(ctx context.Context, orgID int64, actorID uint64, req ReminderGapRecoveryRequest) (*ActionRunResult, error) {
	if req.SourceProtocol == TransactionReminderSource {
		if f == nil || f.deps.CurrentReminderGapRecovery == nil || !f.deps.CurrentReminderGapRecoveryEnabled {
			return nil, errActionsUnavailable()
		}
		if err := ValidateTransactionReminderGap(orgID, actorID, req, false); err != nil {
			return nil, err
		}
		return f.deps.CurrentReminderGapRecovery.AuthorizeReminderGap(ctx, orgID, actorID, req)
	}
	if f == nil || f.deps.ReminderGapRecovery == nil || !f.deps.ReminderGapRecoveryEnabled {
		return nil, errActionsUnavailable()
	}
	if err := ValidateReminderGapRecovery(orgID, actorID, req); err != nil {
		return nil, err
	}
	return f.deps.ReminderGapRecovery.AuthorizeReminderGap(ctx, orgID, actorID, req)
}

func (f *facade) ResolveReminderGap(ctx context.Context, orgID int64, actorID uint64, req ReminderGapRecoveryRequest) (*ActionRunResult, bool, error) {
	if req.SourceProtocol == TransactionReminderSource {
		if f == nil || f.deps.CurrentReminderGapRecovery == nil {
			return nil, false, errActionsUnavailable()
		}
		if err := ValidateTransactionReminderGap(orgID, actorID, req, false); err != nil {
			return nil, false, err
		}
		return f.deps.CurrentReminderGapRecovery.ResolveReminderGap(ctx, orgID, actorID, req)
	}
	if f == nil || f.deps.ReminderGapRecovery == nil {
		return nil, false, errActionsUnavailable()
	}
	if err := ValidateReminderGapRecovery(orgID, actorID, req); err != nil {
		return nil, false, err
	}
	return f.deps.ReminderGapRecovery.ResolveReminderGap(ctx, orgID, actorID, req)
}

func (f *facade) InspectReminderGap(ctx context.Context, orgID int64, actorID uint64, req ReminderGapRecoveryRequest) (*ReminderGapInspection, error) {
	if f == nil || f.deps.CurrentReminderGapRecovery == nil {
		return nil, errActionsUnavailable()
	}
	if err := ValidateTransactionReminderGap(orgID, actorID, req, true); err != nil {
		return nil, err
	}
	return f.deps.CurrentReminderGapRecovery.InspectReminderGap(ctx, orgID, actorID, req)
}

func (f *facade) ResumeReminderGap(ctx context.Context, orgID int64, actorID uint64, req ReminderGapRecoveryRequest) (*ActionRunResult, error) {
	if f == nil || f.deps.CurrentReminderGapRecovery == nil || !f.deps.CurrentReminderGapRecoveryEnabled {
		return nil, errActionsUnavailable()
	}
	if err := ValidateTransactionReminderGap(orgID, actorID, req, false); err != nil {
		return nil, err
	}
	return f.deps.CurrentReminderGapRecovery.ResumeReminderGap(ctx, orgID, actorID, req)
}
