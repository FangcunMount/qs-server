package eventing

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// ReadyIndex is a best-effort Redis ZSet scheduler for pending outbox events.
type ReadyIndex interface {
	Enqueue(ctx context.Context, eventType, eventID string, nextAttemptAt time.Time) error
	Remove(ctx context.Context, eventType, eventID string) error
	RemoveByEventID(ctx context.Context, eventID string) error
	ClaimDueIDs(ctx context.Context, bucket string, limit int, now time.Time) ([]string, error)
}

// PostCommitReadyIndexer backfills the ready index after outbox rows are committed.
type PostCommitReadyIndexer struct {
	index ReadyIndex
}

func NewPostCommitReadyIndexer(index ReadyIndex) *PostCommitReadyIndexer {
	if index == nil {
		return nil
	}
	return &PostCommitReadyIndexer{index: index}
}

func (p *PostCommitReadyIndexer) EnqueueAfterCommit(ctx context.Context, events []event.DomainEvent, nextAttemptAt time.Time) {
	if p == nil || p.index == nil || len(events) == 0 {
		return
	}
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now()
	}
	for _, evt := range events {
		if evt == nil {
			continue
		}
		_ = p.index.Enqueue(ctx, evt.EventType(), evt.EventID(), nextAttemptAt)
	}
}
