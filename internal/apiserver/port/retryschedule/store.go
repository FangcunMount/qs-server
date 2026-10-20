// Package retryschedule defines original-transaction persistence of future
// business authorizations. It is separate from publish Outbox and SDK state.
package retryschedule

import (
	"context"
	"errors"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
)

// ErrReservationRace requires the whole borrowed transaction to roll back.
// Mongo duplicate-key errors cannot be caught and followed by reads in the
// same aborted transaction. Recovery may inspect the winner at a fresh boundary.
var ErrReservationRace = errors.New("retry schedule reservation raced; roll back the original root before recovery")

// Store borrows the host primary and root transaction. SaveFrozen performs
// database operations only, returning the immutable first winner on duplicate
// input. It must never commit the host root or regenerate IDs, clocks or wire.
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
