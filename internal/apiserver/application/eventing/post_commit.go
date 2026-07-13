package eventing

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/pkg/event"
)

// PostCommitDispatcher is the narrow application seam for outbox acceleration.
// Implementations must not imply that a post-commit failure can roll back the
// already committed business transaction.
type PostCommitDispatcher interface {
	AfterCommit(ctx context.Context, events []event.DomainEvent, readyAt time.Time)
}

type readyIndexPostCommitDispatcher struct {
	indexer *PostCommitReadyIndexer
}

func NewReadyIndexPostCommitDispatcher(index ReadyIndex) PostCommitDispatcher {
	return &readyIndexPostCommitDispatcher{indexer: NewPostCommitReadyIndexer(index)}
}

func (d *readyIndexPostCommitDispatcher) AfterCommit(ctx context.Context, events []event.DomainEvent, readyAt time.Time) {
	if d == nil || d.indexer == nil {
		return
	}
	d.indexer.EnqueueAfterCommit(ctx, events, readyAt)
}
