package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"time"
)

// originalMessagingRoot owns this one host SQL root. The SDK guard is first,
// all business/archive writes follow, and all witnesses precede the real commit.
// Both command admission and incoming-event ACKs borrow this same boundary.
func (s *TransactionArchive) originalMessagingRoot(ctx context.Context, e transaction.Execution, write func(*sql.Tx, *ArchiveGuard) error) (transaction.LocalOutcome, error) {
	if !e.Valid() || write == nil {
		return transaction.LocalUnknown, transaction.ErrInvalidPlan
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return transaction.LocalUnknown, err
	}
	defer func() { _ = tx.Rollback() }()
	guard, err := s.Bind(ctx, tx, e) // first operation: fences the same SDK aborter
	if err == nil {
		err = write(tx, guard)
	}
	if err == nil {
		err = guard.CommitWitness(ctx)
	}
	if err != nil {
		// A callback error alone proves nothing. First finish the actual root
		// rollback, then compete for and persist the exact execution abortion.
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return transaction.LocalUnknown, errors.Join(err, rollbackErr)
		}
		abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		aborted, abortErr := s.store.Abort(abortCtx, e)
		if abortErr == nil && aborted {
			return transaction.LocalAborted, err
		}
		return transaction.LocalUnknown, errors.Join(err, abortErr)
	}
	if err = tx.Commit(); err != nil {
		// The server may have committed. Only a fresh exact witness can resolve
		// this; neither cancellation nor a failed second Rollback can.
		return transaction.LocalUnknown, err
	}
	return transaction.LocalCommitted, nil
}
