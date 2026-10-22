package messaging

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

type deferredRecoveryStore interface {
	claim(context.Context, time.Time, time.Duration) (*deferredItem, error)
	renew(context.Context, *deferredItem, time.Time, time.Duration) error
	complete(context.Context, *deferredItem, time.Time) error
	failed(context.Context, *deferredItem, time.Time, deferredFailure) error
}

type DeferredConsumerOptions struct {
	Interval, Lease, RenewEvery, HandlerTimeout time.Duration
	Batch                                       int
}

func DefaultDeferredConsumerOptions(timeout time.Duration) DeferredConsumerOptions {
	return DeferredConsumerOptions{Interval: 5 * time.Second, Lease: time.Minute, RenewEvery: 20 * time.Second, HandlerTimeout: timeout, Batch: 50}
}

type DeferredConsumerStats struct {
	Active            int32
	Completed, Errors uint64
	Closing           bool
}

// DeferredConsumer dispatches the exact already-accepted event, using the host's
// same budgeted dispatcher. It has no publisher, model client or business retry
// authorizer. One loop owns at most one callback and one lease renewal.
type DeferredConsumer struct {
	store               deferredRecoveryStore
	dispatcher          EventDispatcher
	options             DeferredConsumerOptions
	mu                  sync.Mutex
	started, closed     bool
	cancel              context.CancelFunc
	done                chan struct{}
	active              atomic.Int32
	completed, failures atomic.Uint64
}

func NewDeferredConsumer(store *DeferredDeliveryStore, dispatcher EventDispatcher, options DeferredConsumerOptions) (*DeferredConsumer, error) {
	if store == nil {
		return nil, errors.New("deferred delivery store required")
	}
	return newDeferredConsumer(store, dispatcher, options)
}

func newDeferredConsumer(store deferredRecoveryStore, dispatcher EventDispatcher, options DeferredConsumerOptions) (*DeferredConsumer, error) {
	if store == nil || dispatcher == nil || options.Interval <= 0 || options.Lease <= 0 || options.RenewEvery <= 0 || options.RenewEvery >= options.Lease || options.HandlerTimeout <= 0 || options.Batch < 1 || options.Batch > 50 {
		return nil, errors.New("bounded deferred consumer store, dispatcher and lifecycle options required")
	}
	return &DeferredConsumer{store: store, dispatcher: dispatcher, options: options, done: make(chan struct{})}, nil
}

func (r *DeferredConsumer) Start(parent context.Context) error {
	if parent == nil || parent.Err() != nil {
		return errors.New("active deferred consumer parent required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("deferred consumer admission closed")
	}
	if r.started {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.started = true
	go func() {
		defer func() { r.mu.Lock(); r.closed = true; r.mu.Unlock(); close(r.done) }()
		ticker := time.NewTicker(r.options.Interval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			if err := r.scan(ctx); err != nil {
				r.failures.Add(1)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (r *DeferredConsumer) scan(ctx context.Context) error {
	for i := 0; i < r.options.Batch; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := r.store.claim(ctx, time.Now().UTC(), r.options.Lease)
		if err != nil || item == nil {
			return err
		}
		if err := r.consume(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (r *DeferredConsumer) consume(parent context.Context, item *deferredItem) error {
	r.active.Add(1)
	defer r.active.Add(-1)
	work, cancelWork := context.WithTimeout(parent, r.options.HandlerTimeout)
	defer cancelWork()
	// Keep the lease while the REAL callback still runs, including after the
	// caller cancels. A timeout cannot release capacity or imply rollback.
	lease, cancelLease := context.WithCancel(context.WithoutCancel(parent))
	defer cancelLease()
	keeperDone := make(chan struct{})
	var keeperErr error
	go func() {
		defer close(keeperDone)
		ticker := time.NewTicker(r.options.RenewEvery)
		defer ticker.Stop()
		for {
			select {
			case <-lease.Done():
				return
			case <-ticker.C:
				if err := r.store.renew(lease, item, time.Now().UTC(), r.options.Lease); err != nil {
					// Closing the keeper's own context after the callback returns is
					// not an authority failure; real unknown renewals cancel work.
					if lease.Err() == nil {
						keeperErr = err
						cancelWork()
					}
					return
				}
			}
		}
	}()
	var result eventruntime.DispatchResult
	err := work.Err()
	if err == nil {
		result, err = r.dispatcher.DispatchEvent(work, item.EventType, item.Message.Payload)
	}
	cancelLease()
	<-keeperDone // Also joins a driver that has not returned from renewal yet.
	if keeperErr != nil {
		return keeperErr
	}
	final := context.WithoutCancel(parent)
	if err != nil {
		reason := deferredDispatchFailed
		if errors.Is(err, eventruntime.ErrAutomaticRetryPaused) {
			reason = deferredPaused
		}
		return errors.Join(err, r.store.failed(final, item, time.Now().UTC(), reason))
	}
	if result.Outcome != eventruntime.DispatchHandled {
		return r.store.failed(final, item, time.Now().UTC(), deferredUnknownHandler)
	}
	if err := r.store.complete(final, item, time.Now().UTC()); err != nil {
		return err
	}
	r.completed.Add(1)
	return nil
}

func (r *DeferredConsumer) Close(ctx context.Context) error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		if r.cancel != nil {
			r.cancel()
		}
		if !r.started {
			close(r.done)
		}
	}
	r.mu.Unlock()
	return r.Wait(ctx)
}

func (r *DeferredConsumer) Wait(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-r.done:
		return nil
	default:
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		select {
		case <-r.done:
			return nil
		default:
		}
		return &transaction.ShutdownIncomplete{RuntimePending: true, Cause: ctx.Err()}
	}
}

func (r *DeferredConsumer) Stats() DeferredConsumerStats {
	r.mu.Lock()
	closing := r.closed
	r.mu.Unlock()
	return DeferredConsumerStats{Active: r.active.Load(), Completed: r.completed.Load(), Errors: r.failures.Load(), Closing: closing}
}
