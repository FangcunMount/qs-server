package retryschedule

import (
	"bytes"
	"context"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var _ port.HandoffInspector = (*Store)(nil)

func legacyCollectionUUID(ctx context.Context, db *mongo.Database) (primitive.Binary, error) {
	cur, err := db.ListCollections(ctx, bson.M{"name": "rm_outbox"}, options.ListCollections().SetNameOnly(false))
	if err != nil {
		return primitive.Binary{}, err
	}
	defer func() { _ = cur.Close(ctx) }()
	if !cur.Next(ctx) {
		return primitive.Binary{}, transaction.ErrSchema
	}
	var metadata struct {
		Name string `bson:"name"`
		Type string `bson:"type"`
		Info struct {
			UUID primitive.Binary `bson:"uuid"`
		} `bson:"info"`
	}
	if err := cur.Decode(&metadata); err != nil {
		return primitive.Binary{}, err
	}
	if metadata.Name != "rm_outbox" || metadata.Type != "collection" || metadata.Info.UUID.Subtype != 4 || len(metadata.Info.UUID.Data) != 16 || cur.Next(ctx) {
		return primitive.Binary{}, transaction.ErrSchema
	}
	if err := cur.Err(); err != nil {
		return primitive.Binary{}, err
	}
	return metadata.Info.UUID, nil
}

// InspectLegacy performs reads only in a new snapshot transaction, then aborts
// it. Original collection UUID is checked on both sides: a missing, replaced or
// recreated ledger cannot be interpreted as an empty original ledger.
func (s *Store) InspectLegacy(ctx context.Context, selected []model.Frozen) (model.LegacyInventory, error) {
	if mongo.SessionFromContext(ctx) != nil {
		return model.LegacyInventory{}, transaction.ErrTransactionRequired
	}
	if err := model.ValidateHandoffSelection(s.binding, selected); err != nil {
		return model.LegacyInventory{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return model.LegacyInventory{}, err
	}
	defer release()
	before, err := legacyCollectionUUID(ctx, s.primary)
	if err != nil {
		return model.LegacyInventory{}, err
	}
	session, err := s.primary.Client().StartSession()
	if err != nil {
		return model.LegacyInventory{}, err
	}
	defer session.EndSession(ctx)
	if err := session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); err != nil {
		return model.LegacyInventory{}, err
	}
	tx := mongo.NewSessionContext(ctx, session)
	defer func() { _ = session.AbortTransaction(tx) }()
	if err := s.validateIn(tx, s.primary); err != nil {
		return model.LegacyInventory{}, err
	}
	counts := make(map[string]uint64, 6)
	var unconfirmed uint64
	badConfirmation := bson.M{"$and": bson.A{
		bson.M{"$eq": bson.A{"$state", "published"}},
		bson.M{"$or": bson.A{
			bson.M{"$ne": bson.A{bson.M{"$type": "$transport_confirmed_at"}, "date"}},
			bson.M{"$lt": bson.A{"$transport_confirmed_at", time.Unix(0, 0).UTC()}},
		}},
	}}
	cur, err := s.primary.Collection("rm_outbox").Aggregate(tx, mongo.Pipeline{
		bson.D{{Key: "$group", Value: bson.M{"_id": "$state", "count": bson.M{"$sum": 1}, "missing": bson.M{"$sum": bson.M{"$cond": bson.A{badConfirmation, 1, 0}}}}}},
		bson.D{{Key: "$limit", Value: 7}},
	}, options.Aggregate().SetMaxTime(30*time.Second).SetBatchSize(7))
	if err != nil {
		return model.LegacyInventory{}, err
	}
	defer func() { _ = cur.Close(tx) }()
	for cur.Next(tx) {
		var row struct {
			State   string `bson:"_id"`
			Count   uint64 `bson:"count"`
			Missing uint64 `bson:"missing"`
		}
		if err := cur.Decode(&row); err != nil {
			return model.LegacyInventory{}, err
		}
		counts[row.State] = row.Count
		unconfirmed += row.Missing
	}
	if err := cur.Err(); err != nil {
		return model.LegacyInventory{}, err
	}
	if err := cur.Close(tx); err != nil {
		return model.LegacyInventory{}, err
	}
	for _, f := range selected {
		if err := s.inspectHeld(tx, f); err != nil {
			return model.LegacyInventory{}, err
		}
	}
	result, err := model.Inventory(s.binding, counts, uint64(len(selected)), unconfirmed)
	if err != nil {
		return model.LegacyInventory{}, err
	}
	if err := session.AbortTransaction(tx); err != nil {
		return model.LegacyInventory{}, err
	}
	after, err := legacyCollectionUUID(ctx, s.primary)
	if err != nil {
		return model.LegacyInventory{}, err
	}
	if before.Subtype != after.Subtype || !bytes.Equal(before.Data, after.Data) {
		return model.LegacyInventory{}, transaction.ErrSchema
	}
	return result, nil
}

func (s *Store) inspectHeld(ctx context.Context, f model.Frozen) error {
	in := f.Message().Input()
	identity := bson.M{"producer": in.Producer, "message_id": in.ID, "destination": in.Destination}
	count, err := s.primary.Collection("rm_outbox").CountDocuments(ctx, identity, options.Count().SetLimit(2))
	if err != nil {
		return err
	}
	if count != 1 {
		return model.ErrLegacyOwnership
	}
	identity["$expr"] = bson.M{"$lte": bson.A{bson.M{"$binarySize": "$payload"}, 262144}}
	var source legacyHandoffDocument
	if err := s.primary.Collection("rm_outbox").FindOne(ctx, identity).Decode(&source); err != nil {
		return err
	}
	already, err := model.MatchLegacy(f, source.legacySource(), time.Millisecond)
	if err != nil {
		return err
	}
	if !already || source.Version == 0 {
		return model.ErrLegacyOwnership
	}
	key := f.Key()
	var target document
	filter := bson.M{"_id": key[:], "$expr": bson.M{"$and": bson.A{
		bson.M{"$lte": bson.A{bson.M{"$binarySize": "$frozen_plan"}, 1048576}},
		bson.M{"$lte": bson.A{bson.M{"$binarySize": bson.M{"$ifNull": bson.A{"$dispatch_evidence", []byte{}}}}, 1048576}},
	}}}
	if err := s.primary.Collection(collection).FindOne(ctx, filter).Decode(&target); err != nil {
		return err
	}
	r, err := s.decode(target)
	if err != nil {
		return err
	}
	if r.Plan.Digest() != f.Digest() {
		return model.ErrConflict
	}
	return nil
}
