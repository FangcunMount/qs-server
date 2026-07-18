package outbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// ImmediatePublishReader loads a single outbox row eligible for immediate publish.
type ImmediatePublishReader interface {
	GetPublishableEvent(ctx context.Context, eventID string, now time.Time) (PendingEvent, bool, error)
}

// BatchPublisher marks multiple outbox rows in one call.
type BatchPublisher interface {
	MarkEventsPublished(ctx context.Context, eventIDs []string, publishedAt time.Time) error
	MarkEventsFailed(ctx context.Context, failures []FailedMark, nextAttemptAt time.Time) error
}

type FailedMark struct {
	EventID   string
	EventType string
	LastError string
}

// GovernedFailedMark is the durable result of one failed publish transition.
type GovernedFailedMark struct {
	EventID       string
	EventType     string
	AttemptCount  int
	Disposition   retrygovernance.Disposition
	NextAttemptAt *time.Time
}

// GovernedFailureMarker calculates and persists the Outbox-owned retry budget.
// It is optional so legacy Store implementations remain source compatible.
type GovernedFailureMarker interface {
	MarkEventsFailedGoverned(ctx context.Context, failures []FailedMark, failedAt time.Time) ([]GovernedFailedMark, error)
}

// EventIDClaimer claims due outbox rows by explicit event IDs (ready-index path).
type EventIDClaimer interface {
	ClaimEventsByIDs(ctx context.Context, eventIDs []string, now time.Time) ([]PendingEvent, error)
}

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
