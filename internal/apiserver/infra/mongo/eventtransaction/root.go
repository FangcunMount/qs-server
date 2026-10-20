package eventtransaction

import (
	"bytes"
	"context"
	"sync/atomic"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/mongo"
)

type rootKey struct{}
type originalRoot struct {
	owner     *Owner
	session   mongo.Session
	execution transaction.Execution
	closed    atomic.Bool
}

// WithinOriginal binds inside the already-active host session. It never opens
// or commits a root transaction. A driver callback may reenter this function;
// write must only use the already-frozen database values on each invocation.
// Call it only from the SDK local executor after all halves are prepared.
func (o *Owner) WithinOriginal(ctx context.Context, e transaction.Execution, write func(context.Context) error) error {
	if _, nested := ctx.Value(rootKey{}).(*originalRoot); nested || write == nil || !e.Valid() {
		return transaction.ErrTransactionRequired
	}
	if err := o.validatePlan(e.Plan); err != nil {
		return err
	}
	session := mongo.SessionFromContext(ctx)
	if session == nil {
		return transaction.ErrTransactionRequired
	}
	guard, err := o.authority.Bind(mongo.NewSessionContext(ctx, session), e)
	if err != nil {
		return err
	}
	root := &originalRoot{owner: o, session: session, execution: e}
	defer root.closed.Store(true)
	bound := context.WithValue(ctx, rootKey{}, root)
	if err := write(bound); err != nil {
		return err
	}
	return guard.CommitWitness(bound)
}

// RequireOriginal protects V2 repositories from a different database/session,
// stale callback context or another execution's reference. It is not a commit
// decision; the host must still commit the root after all witnesses are saved.
func RequireOriginal(ctx context.Context, db *mongo.Database, proof *evidence.EventEvidenceV2) error {
	root, ok := ctx.Value(rootKey{}).(*originalRoot)
	if !ok || root == nil || root.closed.Load() {
		return transaction.ErrTransactionRequired
	}
	if !root.owner.UsesDatabase(db) || mongo.SessionFromContext(ctx) != root.session {
		return transaction.ErrBinding
	}
	if err := proof.Validate(); err != nil {
		return err
	}
	session, ok := root.session.(mongo.XSession) //nolint:staticcheck // Match the SDK's pinned v1.17.6 active-session bridge.
	if !ok || session.ClientSession() == nil || !session.ClientSession().TransactionRunning() {
		return transaction.ErrTransactionRequired
	}
	return matchReference(root.execution, proof)
}

func matchReference(e transaction.Execution, proof *evidence.EventEvidenceV2) error {
	if err := proof.Validate(); err != nil {
		return err
	}
	for _, branch := range e.Plan.Input().Branches {
		ref, err := e.Plan.Reference(branch.ID, e.Fence, e.WriterEpoch)
		if err != nil {
			return err
		}
		raw, err := transaction.EncodeReference(ref)
		if err == nil && bytes.Equal(raw, proof.TransactionReference) {
			if standard.ReferenceFromMessage(branch.Message) != proof.Reference {
				return transaction.ErrBinding
			}
			return nil
		}
	}
	return transaction.ErrBinding
}

// OriginalPlan exposes the guarded immutable plan only inside the exact active
// original root. It transfers no execution permission or database ownership.
func OriginalPlan(ctx context.Context, db *mongo.Database, proof *evidence.EventEvidenceV2) (transaction.FrozenPlan, error) {
	if err := RequireOriginal(ctx, db, proof); err != nil {
		return transaction.FrozenPlan{}, err
	}
	return ctx.Value(rootKey{}).(*originalRoot).execution.Plan, nil
}
