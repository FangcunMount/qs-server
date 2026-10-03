package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
)

type MessagingBodyReader interface {
	Read(context.Context, *pb.MessagingEnvelope) ([]byte, error)
}

// MessagingEventReceiver is inert until the host calls Receive. It borrows the
// pool, keyring and body client. Only a committed transaction permits NSQ FIN.
type MessagingEventReceiver struct {
	DB     *sql.DB
	Store  *MessagingStore
	Keys   protected.Keyring
	Bodies MessagingBodyReader
	Seal   MessagingSeal
}

func (r *MessagingEventReceiver) transaction(ctx context.Context, f func(context.Context, *sql.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = f(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (r *MessagingEventReceiver) Isolate(ctx context.Context, wire []byte, code string) error {
	return r.transaction(ctx, func(ctx context.Context, tx *sql.Tx) error { return r.Store.Quarantine(ctx, tx, wire, code) })
}
func (r *MessagingEventReceiver) settle(ctx context.Context, e *pb.MessagingEnvelope, raw, wire []byte, held bool) error {
	return r.transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		seal := func(id, org string) (*app.PreparedMessaging, error) {
			outcome := pb.MessagingEventAcknowledgement_STORED
			if held {
				outcome = pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
			}
			body := &pb.MessagingBody{Value: &pb.MessagingBody_EventAcknowledgement{EventAcknowledgement: &pb.MessagingEventAcknowledgement{EventId: e.MessageId, EventBodySha256: e.BodySha256, EventKind: e.Kind, Outcome: outcome}}}
			return r.Seal(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, id, e.AggregateKey, org, "", body)
		}
		var err error
		if !held {
			var count uint64
			count, err = r.Store.FailureAttempts(ctx, tx, e)
			if err != nil {
				return err
			}
			held = count >= 8
		}
		if held {
			return r.Store.ReceiveHeldEvent(ctx, tx, e, raw, wire, seal)
		}
		return r.Store.ReceiveEvent(ctx, tx, e, raw, wire, seal)
	})
}
func (r *MessagingEventReceiver) failure(ctx context.Context, e *pb.MessagingEnvelope, wire []byte) (uint64, error) {
	var attempts uint64
	err := r.transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		attempts, err = r.Store.RecordTechnicalFailure(ctx, tx, e, wire)
		return err
	})
	return attempts, err
}

func (r *MessagingEventReceiver) Receive(ctx context.Context, wire []byte) error {
	e, err := app.AuthenticateMessaging(wire, app.EventsTopic, r.Keys)
	if err != nil {
		return r.Isolate(ctx, wire, "authentication_failed")
	}
	raw, err := r.Bodies.Read(ctx, e)
	if err == nil {
		_, err = app.ParseMessagingBody(e, raw)
		if err == nil {
			err = r.settle(ctx, e, raw, wire, false)
		}
	}
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	} // cancellation is unknown, not a new failure budget
	if errors.Is(err, app.ErrConflict) || errors.Is(err, app.ErrMessagingContract) || errors.Is(err, app.ErrInvalid) {
		return r.Isolate(ctx, wire, "identity_conflict")
	}
	attempts, storageErr := r.failure(ctx, e, wire)
	if errors.Is(storageErr, app.ErrConflict) {
		return r.Isolate(ctx, wire, "identity_conflict")
	}
	if storageErr != nil {
		return errors.New("message failure storage unavailable")
	}
	if attempts >= 8 && len(raw) > 0 {
		if err = r.settle(ctx, e, raw, wire, true); err == nil {
			return nil
		}
		if errors.Is(err, app.ErrConflict) {
			return r.Isolate(ctx, wire, "identity_conflict")
		}
	}
	return errors.New("message receive technically unknown")
}

// Failed never applies a business projection or trusts physical attempts. The
// exact original business identity must authenticate; only local budget can hold.
func (r *MessagingEventReceiver) Failed(ctx context.Context, wire []byte) error {
	return r.failed(ctx, wire, wire)
}

// FailedHandoff keeps physical wrapper bytes distinct from the reconstructed
// signed header. A held Inbox must use the original wire from the local ledger.
func (r *MessagingEventReceiver) FailedHandoff(ctx context.Context, signedWire, auditWire []byte) error {
	return r.failed(ctx, signedWire, auditWire)
}
func (r *MessagingEventReceiver) failed(ctx context.Context, wire, auditWire []byte) error {
	e, err := app.AuthenticateMessaging(wire, app.EventsTopic, r.Keys)
	if err != nil {
		return r.Isolate(ctx, auditWire, "authentication_failed")
	}
	var attempts uint64
	var originalWire []byte
	err = r.transaction(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var err error
		attempts, err = r.Store.FailureAttempts(ctx, tx, e)
		if err == nil && attempts >= 8 {
			err = tx.QueryRowContext(ctx, "SELECT wire FROM ai_messaging_failures WHERE producer=? AND message_id=?", e.Producer, e.MessageId).Scan(&originalWire)
		}
		return err
	})
	if errors.Is(err, app.ErrConflict) {
		return r.Isolate(ctx, auditWire, "identity_conflict")
	}
	if err != nil {
		return errors.New("message failure storage unavailable")
	}
	if attempts >= 8 {
		raw, readErr := r.Bodies.Read(ctx, e)
		if readErr == nil {
			if _, readErr = app.ParseMessagingBody(e, raw); readErr == nil {
				if err = r.settle(ctx, e, raw, originalWire, true); err != nil {
					return err
				}
			}
		}
	}
	return r.Isolate(ctx, auditWire, "failed_delivery")
}
