package eventing

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
)

// PostCommitDispatcher is the narrow application seam for outbox acceleration.
// Implementations must not imply that a post-commit failure can roll back the
// already committed business transaction.
type PostCommitDispatcher interface {
	AfterCommit(ctx context.Context, events []event.DomainEvent, readyAt time.Time)
}
