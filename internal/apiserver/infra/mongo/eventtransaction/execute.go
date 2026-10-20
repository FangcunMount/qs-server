package eventtransaction

import (
	"context"
	"errors"
	"sync"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/mongo"
)

// OriginalRunner is supplied by the host for this business boundary. In
// particular its limiter and transaction callback remain host-owned.
type OriginalRunner interface {
	WithinTransaction(context.Context, func(context.Context) error) error
}

// ExecuteOriginal prepares every half before entering the original runner.
// The SDK executor runs once; the Mongo driver may retry its database callback.
// The caller freezes all bytes/IDs/clocks before invoking this method.
func (o *Owner) ExecuteOriginal(ctx context.Context, e transaction.Execution, runner OriginalRunner, write func(context.Context) error) (transaction.Result, error) {
	if mongo.SessionFromContext(ctx) != nil || runner == nil || write == nil || !e.Valid() {
		return transaction.InitialResult(e), transaction.ErrTransactionRequired
	}
	if err := o.validatePlan(e.Plan); err != nil {
		return transaction.InitialResult(e), err
	}
	var mu sync.Mutex
	started, closed := false, false
	done := make(chan struct{})
	result, err := o.sender.Execute(ctx, e, func(local context.Context, actual transaction.Execution) (transaction.LocalOutcome, error) {
		mu.Lock()
		if closed || started {
			mu.Unlock()
			return transaction.LocalUnknown, transaction.ErrStaleExecution
		}
		started = true
		mu.Unlock()
		defer close(done)
		if actual.Plan.Digest() != e.Plan.Digest() || actual.Fence != e.Fence || actual.WriterEpoch != e.WriterEpoch || actual.Token != e.Token {
			return transaction.LocalUnknown, transaction.ErrStaleExecution
		}
		witnessSaved := false
		if err := runner.WithinTransaction(local, func(tx context.Context) error {
			witnessSaved = false
			if err := o.WithinOriginal(tx, actual, write); err != nil {
				return err
			}
			witnessSaved = true
			return nil
		}); err != nil {
			return transaction.LocalUnknown, err
		}
		if !witnessSaved {
			return transaction.LocalUnknown, transaction.ErrTransactionRequired
		}
		return transaction.LocalCommitted, nil
	})
	mu.Lock()
	closed = true
	join := started
	mu.Unlock()
	if join {
		<-done
	}
	// The fixed SDK derives LocalCommitted from persisted evidence, not from
	// the executor's reported outcome or a runner returning nil.
	if result.Local == transaction.LocalCommitted {
		return result, nil
	}
	if result.Local == transaction.LocalAborted || (!join && result.Local == transaction.LocalNotStarted) {
		return result, errors.Join(port.ErrUnresolved, err)
	}
	// A failed/cancelled commit response is not rollback proof. Read the exact
	// persisted execution from a new boundary without rerunning business work.
	read, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	result.Local = o.Confirm(read, e)
	if result.Local == transaction.LocalCommitted {
		return result, nil
	}
	return result, errors.Join(port.ErrUnresolved, err)
}

func (o *Owner) Confirm(ctx context.Context, e transaction.Execution) transaction.LocalOutcome {
	if mongo.SessionFromContext(ctx) != nil || o.validatePlan(e.Plan) != nil {
		return transaction.LocalUnknown
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	winner, err := o.Lookup(ctx, transaction.RequestFor(e.Plan))
	if err != nil || winner.Execution.Plan.Digest() != e.Plan.Digest() || winner.Execution.Fence != e.Fence || winner.Execution.WriterEpoch != e.WriterEpoch {
		return transaction.LocalUnknown
	}
	refs := e.References()
	facts := make([]*transaction.Evidence, 0, len(refs))
	for _, ref := range refs {
		fact, err := o.Evidence(ctx, ref)
		if err != nil {
			return transaction.LocalUnknown
		}
		facts = append(facts, fact)
	}
	local := transaction.DecideLocal(refs, facts)
	if (local == transaction.LocalCommitted && winner.State != transaction.Committed) || (local == transaction.LocalAborted && winner.State != transaction.Aborted) {
		return transaction.LocalUnknown
	}
	return local
}
