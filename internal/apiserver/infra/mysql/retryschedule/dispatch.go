package retryschedule

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

var _ port.DispatchStore = (*Store)(nil)

type dispatchGuardKey struct{}
type dispatchGuard struct {
	store  *Store
	record model.Record
	proof  []byte
	used   atomic.Bool
}

func (s *Store) GuardDispatch(ctx context.Context, r model.Record, proof *evidence.EventEvidenceV2) (context.Context, error) {
	if r.Validate() != nil || r.State != model.Ready || r.Plan.ValidateDispatchProof(proof) != nil {
		return nil, model.ErrInvalid
	}
	plan, err := guarded.OriginalPlan(ctx, s.primary, proof)
	if err != nil {
		return nil, err
	}
	if _, err := model.ReadDispatch(r, plan); err != nil {
		return nil, err
	}
	tx, err := s.root(ctx)
	if err != nil {
		return nil, err
	}
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.validateIn(ctx, tx, true); err != nil {
		return nil, err
	}
	key := r.Plan.Key()
	actual, err := s.readRow(tx.QueryRowContext(ctx, "SELECT "+columns+" FROM qs_retry_schedule WHERE schedule_key=? FOR UPDATE", key[:]))
	if err != nil {
		return nil, err
	}
	if actual.Plan.Digest() != r.Plan.Digest() || actual.Version != r.Version || actual.State != model.Ready {
		return nil, transaction.ErrStaleExecution
	}
	raw, err := json.Marshal(proof)
	if err != nil {
		return nil, err
	}
	return context.WithValue(ctx, dispatchGuardKey{}, &dispatchGuard{store: s, record: r, proof: raw}), nil
}

func (s *Store) MarkDispatched(ctx context.Context, r model.Record, proof *evidence.EventEvidenceV2) error {
	guard, ok := ctx.Value(dispatchGuardKey{}).(*dispatchGuard)
	if !ok || guard == nil || guard.store != s || guard.record.Plan.Digest() != r.Plan.Digest() || guard.record.Version != r.Version {
		return transaction.ErrTransactionRequired
	}
	if _, err := guarded.OriginalPlan(ctx, s.primary, proof); err != nil {
		return err
	}
	raw, err := json.Marshal(proof)
	if err != nil || !bytes.Equal(raw, guard.proof) {
		return transaction.ErrBinding
	}
	if !guard.used.CompareAndSwap(false, true) {
		return transaction.ErrStaleExecution
	}
	tx, err := s.root(ctx)
	if err != nil {
		return err
	}
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return err
	}
	defer release()
	key, digest := r.Plan.Key(), r.Plan.Digest()
	result, err := tx.ExecContext(ctx, "UPDATE qs_retry_schedule SET state=?,revision=revision+1,dispatch_evidence=? WHERE schedule_key=? AND plan_hash=? AND revision=? AND state=?", []byte(model.DispatchCommitted), raw, key[:], digest[:], r.Version, []byte(model.Ready))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return transaction.ErrStaleExecution
	}
	return nil
}
