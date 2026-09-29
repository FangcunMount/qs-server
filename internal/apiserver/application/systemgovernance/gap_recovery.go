package systemgovernance

import (
	"context"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// GapRecoveryRequest names one reviewed, original assessment event. The
// organization and actor are taken only from the protected request context.
type GapRecoveryRequest struct {
	RequestID       string    `json:"request_id"`
	AssessmentID    uint64    `json:"assessment_id"`
	EventID         string    `json:"event_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	Reason          string    `json:"reason"`
	SubmittedBefore time.Time `json:"submitted_before"`
	Confirm         bool      `json:"confirm"`
}

type GapRecoveryDecision struct {
	Authorized          bool   `json:"authorized"`
	Code                string `json:"code"`
	OutboxVersionBefore uint64 `json:"outbox_version_before,omitempty"`
	OutboxVersionAfter  uint64 `json:"outbox_version_after,omitempty"`
}

// GapRecoveryStore commits the host request ledger and original Outbox
// requeue in one transaction. It must not publish directly.
type GapRecoveryStore interface {
	AuthorizeGapRecovery(context.Context, int64, uint64, GapRecoveryRequest) (*GapRecoveryDecision, error)
	ResolveGapRecovery(context.Context, int64, uint64, GapRecoveryRequest) (*GapRecoveryDecision, bool, error)
}

func validateGapRecoveryRequest(orgID int64, actorUserID uint64, req GapRecoveryRequest) error {
	if orgID <= 0 || actorUserID == 0 || req.AssessmentID == 0 || req.ExpectedVersion == 0 ||
		strings.TrimSpace(req.RequestID) == "" || len(req.RequestID) > 64 ||
		strings.TrimSpace(req.EventID) == "" || len(req.EventID) > 128 ||
		strings.TrimSpace(req.Reason) == "" || len(req.Reason) > 1024 ||
		req.SubmittedBefore.IsZero() || req.SubmittedBefore.After(time.Now().Add(-10*time.Minute)) {
		return errors.WithCode(code.ErrInvalidArgument, "original event identity, stable request ID and review reason are required")
	}
	return nil
}

func (f *facade) AuthorizeGapRecovery(ctx context.Context, orgID int64, actorUserID uint64, req GapRecoveryRequest) (*GapRecoveryDecision, error) {
	if f == nil || f.deps.GapRecoveryStore == nil {
		return nil, errActionsUnavailable()
	}
	if err := validateGapRecoveryRequest(orgID, actorUserID, req); err != nil {
		return nil, err
	}
	if !req.Confirm {
		return nil, errors.WithCode(code.ErrInvalidArgument, "explicit confirmation is required")
	}
	return f.deps.GapRecoveryStore.AuthorizeGapRecovery(ctx, orgID, actorUserID, req)
}

// Resolve only reads the original request decision after an unknown response.
// A missing receipt never authorizes a new request ID.
func (f *facade) ResolveGapRecovery(ctx context.Context, orgID int64, actorUserID uint64, req GapRecoveryRequest) (*GapRecoveryDecision, bool, error) {
	if f == nil || f.deps.GapRecoveryStore == nil {
		return nil, false, errActionsUnavailable()
	}
	if err := validateGapRecoveryRequest(orgID, actorUserID, req); err != nil {
		return nil, false, err
	}
	return f.deps.GapRecoveryStore.ResolveGapRecovery(ctx, orgID, actorUserID, req)
}
