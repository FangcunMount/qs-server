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
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

var ErrBusy = errors.New("retry dispatch scan already in progress")

// Dispatcher is inert. The host admits and joins Once calls before closing the
// borrowed owner/database. A scan is serial, bounded to100 and never spawns work.
type Dispatcher struct {
	store            port.DispatchStore
	owner            events.Committer
	authority        port.AuthorityValidator
	active           atomic.Bool
	disableAutomatic func() bool
}

type Outcome struct {
	ScheduleID  string
	State       model.State
	Transaction transaction.Result
	Err         error
}

func New(store port.DispatchStore, owner events.Committer, authority port.AuthorityValidator, options ...Option) (*Dispatcher, error) {
	if store == nil || owner == nil || authority == nil {
		return nil, transaction.ErrBinding
	}
	d := &Dispatcher{store: store, owner: owner, authority: authority}
	for _, option := range options {
		if option != nil {
			option(d)
		}
	}
	return d, nil
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
	if d.automaticPaused(r) {
		out.Err = eventruntime.ErrAutomaticRetryPaused
		return out
	}
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
	return d.executeReservation(ctx, r, reservation)
}

func (d *Dispatcher) executeReservation(ctx context.Context, r model.Record, reservation transaction.Reservation) Outcome {
	out := Outcome{ScheduleID: r.Plan.ID(), State: r.State}
	execution := reservation.Execution
	proof, err := model.DispatchProof(r, execution, d.owner.Artifact)
	if err != nil {
		out.Err = err
		return out
	}
	if !reservation.Acquired {
		// A duplicate/unknown does not authorize a second callback or new fence.
		// Explicit aborted-dispatch retry authorization is a separate host action.
		out.Transaction = transaction.InitialResult(execution)
		out.Transaction.Local = d.owner.Confirm(ctx, execution)
		return d.recover(ctx, r, proof, out)
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
	return d.recover(ctx, r, proof, out)
}

func (d *Dispatcher) recover(ctx context.Context, r model.Record, proof *evidence.EventEvidenceV2, out Outcome) Outcome {
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
	if actual.Validate() != nil || actual.Plan.Digest() != r.Plan.Digest() || actual.Version != r.Version+1 || actual.State != model.DispatchCommitted || actual.Proof == nil || !bytes.Equal(actual.Proof.TransactionReference, proof.TransactionReference) || !actual.Proof.RecordedAt.Equal(proof.RecordedAt) {
		out.Err = events.ErrUnresolved
		return out
	}
	out.State = actual.State
	out.Err = nil
	return out
}

type Option func(*Dispatcher)

// WithAutomaticRetrySwitch shares the original emergency switch. Only
// automatic work is paused; no half, new fence, deadline or attempt is created.
func WithAutomaticRetrySwitch(disabled func() bool) Option {
	return func(d *Dispatcher) { d.disableAutomatic = disabled }
}

func (d *Dispatcher) automaticPaused(r model.Record) bool {
	return r.Plan.Authority().Origin == retrygovernance.AttemptOriginAutomatic && d.disableAutomatic != nil && d.disableAutomatic()
}

// RetryAborted is a separate, explicit host maintenance action. Once and
// Checker never call it implicitly. The host joins it before closing resources.
func (d *Dispatcher) RetryAborted(ctx context.Context, r model.Record, previous transaction.Execution, lease time.Duration) Outcome {
	out := Outcome{ScheduleID: r.Plan.ID(), State: r.State}
	if !d.active.CompareAndSwap(false, true) {
		out.Err = ErrBusy
		return out
	}
	defer d.active.Store(false)
	if r.Validate() != nil || r.State != model.Ready || !(transaction.ReserveOptions{Lease: lease}).Valid() {
		out.Err = model.ErrInvalid
		return out
	}
	if d.automaticPaused(r) {
		out.Err = eventruntime.ErrAutomaticRetryPaused
		return out
	}
	if _, err := model.ReadDispatch(r, previous.Plan, d.owner.Artifact); err != nil {
		out.Err = err
		return out
	}
	read, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := d.store.Validate(read); err != nil {
		out.Err = err
		return out
	}
	fresh, err := d.store.Find(read, r.Plan)
	if err != nil {
		out.Err = err
		return out
	}
	if fresh == nil || fresh.Validate() != nil || fresh.State != model.Ready || fresh.Version != r.Version || fresh.Plan.Digest() != r.Plan.Digest() {
		out.Err = transaction.ErrStaleExecution
		return out
	}
	if d.owner.Confirm(read, previous) != transaction.LocalAborted {
		out.Err = transaction.ErrStaleExecution
		return out
	}
	retry, ok := d.owner.(events.RetryCommitter)
	if !ok {
		out.Err = transaction.ErrBinding
		return out
	}
	reservation, err := retry.RetryAborted(ctx, previous, transaction.ReserveOptions{Lease: lease})
	if err != nil {
		out.Err = err
		return out
	}
	next := reservation.Execution
	if !reservation.Acquired || !next.Valid() || next.Plan.Digest() != previous.Plan.Digest() || next.Fence != previous.Fence+1 || next.WriterEpoch != previous.WriterEpoch {
		out.Err = transaction.ErrStaleExecution
		return out
	}
	return d.executeReservation(ctx, r, reservation)
}
