package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"time"

	durable "github.com/FangcunMount/reliable-messaging/delivery/mysql"
	"github.com/FangcunMount/reliable-messaging/transport"
)

var ErrMessagingRelayActive = errors.New("AI messaging relay step already active")

// RawMessagingPublisher borrows an explicitly started host publisher. Its result
// describes Broker acceptance only, never the receiver's business transaction.
type RawMessagingPublisher interface {
	PublishRaw(context.Context, string, []byte) transport.Result
}

// MessagingRelay performs one bounded step for the existing host supervisor.
// It owns no pool, client, timer or goroutine. One host process must own delivery;
// the local gate prevents overlapping steps, not distributed ownership.
type MessagingRelay struct {
	db        *sql.DB
	outbox    *durable.Outbox
	publisher RawMessagingPublisher
	active    atomic.Bool
}

func NewMessagingRelay(db *sql.DB, outbox *durable.Outbox, publisher RawMessagingPublisher) (*MessagingRelay, error) {
	if db == nil || outbox == nil || publisher == nil {
		return nil, errors.New("host storage and publisher required")
	}
	return &MessagingRelay{db: db, outbox: outbox, publisher: publisher}, nil
}

// Step reads persisted wire without holding a transaction during network I/O.
// Cancellation leaves unrecorded outcomes eligible for an identical re-PUB.
// Storage errors propagate; they cannot be presented as successful settlement.
func (r *MessagingRelay) Step(ctx context.Context) error {
	if !r.active.CompareAndSwap(false, true) {
		return ErrMessagingRelayActive
	}
	defer r.active.Store(false)
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	records, readErr := r.outbox.Pending(ctx, tx, 20)
	rollbackErr := tx.Rollback()
	if readErr != nil {
		return readErr
	}
	if rollbackErr != nil {
		return rollbackErr
	}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return err
		}
		if record.Attempts >= 8 {
			if err := r.settle(ctx, record, transport.Result{}, true); err != nil {
				return err
			}
			continue
		}
		publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		result := r.publisher.PublishRaw(publishCtx, record.Topic, record.Wire)
		cancel()
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := r.settle(ctx, record, result, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *MessagingRelay) settle(ctx context.Context, record durable.Record, result transport.Result, exhausted bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	switch {
	case exhausted:
		err = r.outbox.Hold(ctx, tx, record.Identity, record.BodySHA256, "publish_budget_exhausted")
	case result.Outcome == transport.Confirmed:
		err = r.outbox.Published(ctx, tx, record.Identity, record.BodySHA256, 30)
	case result.Outcome == transport.Rejected:
		err = r.outbox.Hold(ctx, tx, record.Identity, record.BodySHA256, "publish_rejected")
	default:
		// Bounded technical retry does not grant permission to repeat model execution.
		delay := 1 << min(record.Attempts+1, uint64(6))
		err = r.outbox.Retry(ctx, tx, record.Identity, record.BodySHA256, min(delay, 60), "publish_unknown")
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
