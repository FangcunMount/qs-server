// Package retryschedule defines original-transaction persistence of future
// business authorizations. It is separate from publish Outbox and SDK state.
package retryschedule

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// ErrReservationRace requires the whole borrowed transaction to roll back.
// Mongo duplicate-key errors cannot be caught and followed by reads in the
// same aborted transaction. Recovery may inspect the winner at a fresh boundary.
var ErrReservationRace = errors.New("retry schedule reservation raced; roll back the original root before recovery")

// Store borrows the host primary and root transaction. SaveFrozen performs
// database operations only, returning the immutable Ready first winner on duplicate
// input while competing with dispatch/governance; terminal plans are read with Find. It must never commit the host root or regenerate IDs, clocks or wire.
type Store interface {
	Validate(context.Context) error
	SaveFrozen(context.Context, model.Frozen) (model.Frozen, error)
	Find(context.Context, model.Frozen) (*model.Record, error)
	Due(context.Context, time.Time, int) ([]model.Record, error)
}

// DispatchStore competes with governance over the same schedule inside the
// SDK-guarded original root. The returned context cannot escape that callback.
type DispatchStore interface {
	Store
	GuardDispatch(context.Context, model.Record, *evidence.EventEvidenceV2) (context.Context, error)
	MarkDispatched(context.Context, model.Record, *evidence.EventEvidenceV2) error
}

// AuthorityValidator rechecks the original Run's current decision in the same
// database root after the schedule guard. It never executes business work.
type AuthorityValidator interface {
	ValidateDispatchAuthority(context.Context, model.Frozen) error
}

// PlanBuilder freezes a complete first message before the original root.
type PlanBuilder interface {
	Freeze(model.Authority, event.DomainEvent, *retrygovernance.Decision, any) (model.Frozen, error)
}

// AuthorizationGuard rechecks the original aggregate in the already-active
// business root and competes with a new business attempt. It performs DB I/O only.
type AuthorizationGuard interface {
	GuardAuthorization(context.Context, model.Frozen) error
}

// HandoffStore borrows an explicitly opened host root during a controlled
// offline handoff. It neither commits nor prepares messages. Any error requires
// the caller to roll back the whole root. Already-transferred calls only verify
// the original source and exact stored plan; they never reset terminal state.
type HandoffStore interface {
	Store
	Binding() transaction.Binding
	TransferLegacy(context.Context, model.Frozen, AuthorityValidator) (alreadyTransferred bool, err error)
}

// HandoffInspector owns one bounded fresh read boundary on the configured
// original primary. Selected held sources must match their exact stored plans;
// unselected held rows remain unaccounted. It never transfers or publishes.
type HandoffInspector interface {
	InspectLegacy(context.Context, []model.Frozen) (model.LegacyInventory, error)
}
