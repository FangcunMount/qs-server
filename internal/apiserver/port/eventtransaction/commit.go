// Package eventtransaction describes a host-owned original-root commit. It is
// independent of the historical Outbox staging/relay protocol.
package eventtransaction

import (
	"context"
	"errors"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

var ErrUnresolved = errors.New("event transaction unresolved; recover the original operation")

// Request separates stable business input from generated IDs, clocks and the
// first immutable artifact. Artifact must preserve raw business bytes (for
// example using []byte fields rather than marshaling embedded RawMessage).
type Request struct {
	Key       string
	InputHash [32]byte
	Artifact  []byte
	Events    []event.DomainEvent
	// Prepared carries canonical QS domain inputs frozen by a durable business
	// schedule. It is mutually exclusive with Events; original wire is reused.
	Prepared []message.Input
}

// Committer freezes/reserves before Broker I/O, then prepares all halves before
// opening the host root. Write performs database operations only. It is never
// called for a reservation loser, for recovery, or by a Checker.
type Committer interface {
	Reserve(context.Context, Request) (transaction.Reservation, error)
	// Find reads an existing plan by exact host scope/key, without creating an
	// execution or accepting a caller's invented hash for an unknown result.
	Find(context.Context, string, string) (transaction.Reservation, error)
	Artifact(transaction.FrozenPlan) ([]byte, error)
	Commit(context.Context, transaction.Execution, func(context.Context) error) (transaction.Result, error)
	Confirm(context.Context, transaction.Execution) transaction.LocalOutcome
}
