package outbox

import (
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// ReferencePreparer prepares the exact immutable message reference without
// opening a transaction or borrowing a connection.
type ReferencePreparer interface {
	PrepareReference(event.DomainEvent) (evidence.StandardReference, error)
}
