package retryschedule

import (
	"bytes"
	"context"
	"encoding/json"
	"sync/atomic"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	guarded "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ port.DispatchStore = (*Store)(nil)

type dispatchGuardKey struct{}
type dispatchGuard struct {
	store    *Store
	record   model.Record
	proof    []byte
	revision int64
	used     atomic.Bool
}

func (s *Store) GuardDispatch(ctx context.Context, r model.Record, proof *evidence.EventEvidenceV2) (context.Context, error) {
	if r.Validate() != nil || r.State != model.Ready || r.Plan.ValidateDispatchProof(proof) != nil {
		return nil, model.ErrInvalid
	}
	plan, err := guarded.OriginalPlan(ctx, s.db, proof)
	if err != nil {
		return nil, err
	}
	if _, err := model.ReadDispatch(r, plan); err != nil {
		return nil, err
	}
	if err := s.root(ctx); err != nil {
		return nil, err
	}
	if err := s.validateIn(ctx, s.db); err != nil {
		return nil, err
	}
	key, digest := r.Plan.Key(), r.Plan.Digest()
	filter := bson.M{"_id": key[:], "owner": s.binding.Owner, "plan_hash": digest[:], "state": model.Ready, "revision": r.Version, "guard_version": bson.M{"$lt": int64(1<<63 - 1)}}
	var d document
	err = s.db.Collection(collection).FindOneAndUpdate(ctx, filter, bson.M{"$inc": bson.M{"guard_version": int64(1)}}, options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&d)
	if err == mongo.ErrNoDocuments {
		return nil, transaction.ErrStaleExecution
	}
	if err != nil {
		return nil, err
	}
	actual, err := s.decode(d)
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
	return context.WithValue(ctx, dispatchGuardKey{}, &dispatchGuard{store: s, record: r, proof: raw, revision: d.GuardVersion}), nil
}

func (s *Store) MarkDispatched(ctx context.Context, r model.Record, proof *evidence.EventEvidenceV2) error {
	guard, ok := ctx.Value(dispatchGuardKey{}).(*dispatchGuard)
	if !ok || guard == nil || guard.store != s || guard.record.Plan.Digest() != r.Plan.Digest() || guard.record.Version != r.Version {
		return transaction.ErrTransactionRequired
	}
	if _, err := guarded.OriginalPlan(ctx, s.db, proof); err != nil {
		return err
	}
	raw, err := json.Marshal(proof)
	if err != nil || !bytes.Equal(raw, guard.proof) {
		return transaction.ErrBinding
	}
	if !guard.used.CompareAndSwap(false, true) {
		return transaction.ErrStaleExecution
	}
	if err := s.root(ctx); err != nil {
		return err
	}
	key, digest := r.Plan.Key(), r.Plan.Digest()
	filter := bson.M{"_id": key[:], "owner": s.binding.Owner, "plan_hash": digest[:], "state": model.Ready, "revision": r.Version, "guard_version": guard.revision}
	result, err := s.db.Collection(collection).UpdateOne(ctx, filter, bson.M{"$set": bson.M{"state": model.DispatchCommitted, "dispatch_evidence": raw}, "$inc": bson.M{"revision": int64(1)}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return transaction.ErrStaleExecution
	}
	return nil
}
