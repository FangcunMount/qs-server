package outbox

import (
	"context"
	"time"
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

// EventIDClaimer claims due outbox rows by explicit event IDs (ready-index path).
type EventIDClaimer interface {
	ClaimEventsByIDs(ctx context.Context, eventIDs []string, now time.Time) ([]PendingEvent, error)
}
