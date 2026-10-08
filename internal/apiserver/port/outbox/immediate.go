package outbox

import (
	"context"
	"errors"
	"time"
)

type ManualReplayTarget struct {
	EventID              string `json:"event_id"`
	ExpectedAttemptCount int    `json:"expected_attempt_count"`
}

type ManualReplayResult struct {
	EventID    string `json:"event_id"`
	Authorized bool   `json:"authorized"`
	Reason     string `json:"reason,omitempty"`
}

// ManualReplayAuthorizer grants each exhausted outbox row exactly one
// additional publish attempt without resetting attempt_count or event_id.
type ManualReplayAuthorizer interface {
	AuthorizeManualReplay(ctx context.Context, orgID int64, requestID string, targets []ManualReplayTarget, authorizedAt time.Time) ([]ManualReplayResult, error)
}

// DurableManualReplayAuthorizer binds the operator's complete request to a
// durable authorization ledger. The reason is part of request identity, so a
// repeated request ID with changed approval input must be rejected.
type DurableManualReplayAuthorizer interface {
	AuthorizeManualReplayWithReason(ctx context.Context, orgID int64, requestID, reason string, targets []ManualReplayTarget) ([]ManualReplayResult, error)
}

// ErrManualReplayOutcomeUnknown means a durable authorization may have
// committed despite an error. The governance audit must remain recoverable;
// the caller must not complete it as failed or issue another request ID.
var ErrManualReplayOutcomeUnknown = errors.New("manual replay outcome awaits durable reconciliation")
