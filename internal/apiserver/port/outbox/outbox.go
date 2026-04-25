package outbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// PendingEvent represents a claimed outbox row that is ready to publish.
type PendingEvent struct {
	EventID string
	Event   event.DomainEvent
}

// Store manages due outbox rows.
type Store interface {
	ClaimDueEvents(ctx context.Context, limit int, now time.Time) ([]PendingEvent, error)
	MarkEventPublished(ctx context.Context, eventID string, publishedAt time.Time) error
	MarkEventFailed(ctx context.Context, eventID, lastError string, nextAttemptAt time.Time) error
}

// StatusBucket describes one unfinished outbox status bucket.
type StatusBucket struct {
	Status           string     `json:"status"`
	Count            int64      `json:"count"`
	OldestCreatedAt  *time.Time `json:"oldest_created_at,omitempty"`
	OldestAgeSeconds float64    `json:"oldest_age_seconds"`
}

// StatusSnapshot is a DB-neutral read-only view of unfinished outbox work.
type StatusSnapshot struct {
	Store       string         `json:"store"`
	GeneratedAt time.Time      `json:"generated_at"`
	Buckets     []StatusBucket `json:"buckets"`
}

// StatusReader exposes read-only outbox backlog/lag state.
type StatusReader interface {
	OutboxStatusSnapshot(ctx context.Context, now time.Time) (StatusSnapshot, error)
}
