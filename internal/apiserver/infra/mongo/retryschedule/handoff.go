package retryschedule

import (
	"context"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ port.HandoffStore = (*Store)(nil)

// RawValue retains null/missing distinctions without trusting a decoded zero.
func fieldPresent(value bson.RawValue) bool { return value.Type != 0 && value.Type != bson.TypeNull }
func (s *Store) TransferLegacy(ctx context.Context, f model.Frozen, authority port.AuthorityValidator) (bool, error) {
	if !f.Valid() || f.Authority().Owner != s.binding.Owner || authority == nil {
		return false, transaction.ErrBinding
	}
	if err := s.root(ctx); err != nil {
		return false, err
	}
	if err := s.validateIn(ctx, s.db); err != nil {
		return false, err
	}
	ctx, cancel := context.WithTimeout(ctx, readBudget)
	defer cancel()
	in := f.Message().Input()
	cursor, err := s.db.Collection("rm_outbox").Find(ctx, bson.M{"producer": in.Producer, "message_id": in.ID, "destination": in.Destination}, options.Find().SetLimit(2))
	if err != nil {
		return false, err
	}
	var found []legacyHandoffDocument
	if err = cursor.All(ctx, &found); err != nil {
		return false, err
	}
	if len(found) != 1 {
		return false, model.ErrLegacyOwnership
	}
	r := found[0]
	row := r.legacySource()
	already, err := model.MatchLegacy(f, row, time.Millisecond)
	if err != nil {
		return false, err
	}
	if already {
		key := f.Key()
		var d document
		if err = s.db.Collection(collection).FindOne(ctx, bson.M{"_id": key[:]}).Decode(&d); err != nil {
			return false, err
		}
		record, err := s.decode(d)
		if err != nil {
			return false, err
		}
		if record.Plan.Digest() != f.Digest() {
			return false, model.ErrConflict
		}
		return true, nil
	}
	winner, err := s.SaveFrozen(ctx, f)
	if err != nil {
		return false, err
	}
	if winner.Digest() != f.Digest() {
		return false, model.ErrConflict
	}
	if err = authority.ValidateDispatchAuthority(ctx, f); err != nil {
		return false, err
	}
	filter := bson.M{"_id": r.ID, "version": int64(r.Version), "state": "pending", "claim_token": nil, "lease_until": nil, "transport_confirmed_at": nil, "manual_replay_request_id": nil, "manual_replay_version": nil, "$expr": bson.M{"$and": bson.A{
		bson.M{"$gt": bson.A{"$next_attempt_at", "$$NOW"}},
		bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$attempt_count", int64(0)}}, int64(0)}},
		bson.M{"$eq": bson.A{bson.M{"$ifNull": bson.A{"$failure_count", int64(0)}}, int64(0)}},
	}}}
	changed, err := s.db.Collection("rm_outbox").UpdateOne(ctx, filter, bson.M{"$set": bson.M{"state": "held", "last_error_code": model.HandoffMarker(f)}, "$inc": bson.M{"version": int64(1)}})
	if err != nil {
		return false, err
	}
	if changed.MatchedCount != 1 {
		return false, model.ErrLegacyOwnership
	}
	return false, nil
}
