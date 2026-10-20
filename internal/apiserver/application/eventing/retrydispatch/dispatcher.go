// Package retrydispatch delivers already-authorized due events. It has no
// evaluator, report builder, model client or business-attempt writer.
package retrydispatch

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	events "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

var ErrBusy = errors.New("retry dispatch scan already in progress")

// Dispatcher is inert. The host admits and joins Once calls before closing the
// borrowed owner/database. A scan is serial, bounded to100 and never spawns work.
type Dispatcher struct {
	store     port.DispatchStore
	owner     events.Committer
	authority port.AuthorityValidator
	active    atomic.Bool
}

type Outcome struct {
	ScheduleID  string
	State       model.State
	Transaction transaction.Result
	Err         error
}

func New(store port.DispatchStore, owner events.Committer, authority port.AuthorityValidator) (*Dispatcher, error) {
	if store == nil || owner == nil || authority == nil {
		return nil, transaction.ErrBinding
	}
	return &Dispatcher{store: store, owner: owner, authority: authority}, nil
}

func (d *Dispatcher) Once(ctx context.Context, at time.Time, limit int) ([]Outcome, error) {
	if at.IsZero() || limit < 1 || limit > 100 {
		return nil, model.ErrInvalid
	}
	if !d.active.CompareAndSwap(false, true) {
		return nil, ErrBusy
	}
	defer d.active.Store(false)
	if err := d.store.Validate(ctx); err != nil {
		return nil, err
	}
	records, err := d.store.Due(ctx, at, limit)
	if err != nil {
		return nil, err
	}
	if len(records) > limit {
		return nil, model.ErrInvalid
	}
	outcomes := make([]Outcome, 0, len(records))
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return outcomes, err
		}
		outcome := d.dispatch(ctx, record, at)
		outcomes = append(outcomes, outcome)
	}
	return outcomes, nil
}

func (d *Dispatcher) dispatch(ctx context.Context, r model.Record, at time.Time) Outcome {
	out := Outcome{ScheduleID: r.Plan.ID(), State: r.State}
	request, err := model.FreezeDispatch(r, at)
	if err != nil {
		out.Err = err
		return out
	}
	reservation, err := d.owner.Reserve(ctx, request)
	if err != nil {
		out.Err = err
		return out
	}
	execution := reservation.Execution
	proof, err := model.DispatchProof(r, execution)
	if err != nil {
		out.Err = err
		return out
	}
	if !reservation.Acquired {
		// A duplicate/unknown does not authorize a second callback or new fence.
		// Explicit aborted-dispatch retry authorization is a separate host action.
		out.Transaction = transaction.InitialResult(execution)
		out.Transaction.Local = d.owner.Confirm(ctx, execution)
		return d.recover(ctx, r, proof.TransactionReference, out)
	}
	out.Transaction, err = d.owner.Commit(ctx, execution, func(root context.Context) error {
		guarded, err := d.store.GuardDispatch(root, r, proof)
		if err != nil {
			return err
		}
		if err := d.authority.ValidateDispatchAuthority(guarded, r.Plan); err != nil {
			return err
		}
		return d.store.MarkDispatched(guarded, r, proof)
	})
	if out.Transaction.Local != transaction.LocalCommitted {
		out.Err = errors.Join(events.ErrUnresolved, err)
		return out
	}
	return d.recover(ctx, r, proof.TransactionReference, out)
}

func (d *Dispatcher) recover(ctx context.Context, r model.Record, reference []byte, out Outcome) Outcome {
	if out.Transaction.Local != transaction.LocalCommitted {
		out.Err = events.ErrUnresolved
		return out
	}
	read, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	actual, err := d.store.Find(read, r.Plan)
	if err != nil {
		out.Err = errors.Join(events.ErrUnresolved, err)
		return out
	}
	if actual.Validate() != nil || actual.Plan.Digest() != r.Plan.Digest() || actual.Version != r.Version+1 || actual.State != model.DispatchCommitted || actual.Proof == nil || !bytes.Equal(actual.Proof.TransactionReference, reference) {
		out.Err = events.ErrUnresolved
		return out
	}
	out.State = actual.State
	out.Err = nil
	return out
}
