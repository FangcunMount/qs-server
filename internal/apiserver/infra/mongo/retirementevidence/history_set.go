package retirementevidence

import (
	"bytes"
	"context"
	"fmt"
	"reflect"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HistoricalSetSnapshot is a point-read CAS for a separate maintenance slot.
// Business BSON is exact and immutable; the caller owns the active transaction.
type HistoricalSetSnapshot struct {
	snapshot *Snapshot
	initial  *evidence.HistoricalReferenceSetV1
}

func ReadHistoricalSet(ctx context.Context, collection *mongo.Collection, filter any, slot string) (*HistoricalSetSnapshot, error) {
	if collection == nil || (slot != "legacy_submission_evidence" && slot != "historical_generated_evidence") {
		return nil, ErrUnverifiable
	}
	if err := requireHistoricalUniqueIndex(ctx, collection, slot); err != nil {
		return nil, err
	}
	var raw bson.Raw
	if err := collection.FindOne(ctx, filter, options.FindOne().SetCollation(&options.Collation{Locale: "simple"})).Decode(&raw); err != nil {
		return nil, err
	}
	base, set, err := splitHistoricalSet(raw, slot)
	if err != nil {
		return nil, err
	}
	return &HistoricalSetSnapshot{snapshot: &Snapshot{raw: append(bson.Raw(nil), raw...), baseline: base, path: []string{slot}, client: collection.Database().Client(), database: collection.Database().Name(), collection: collection.Name()}, initial: set}, nil
}

func requireHistoricalUniqueIndex(ctx context.Context, collection *mongo.Collection, slot string) error {
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}
	var indexes []struct {
		Key       bson.D `bson:"key"`
		Unique    bool   `bson:"unique"`
		Partial   bson.D `bson:"partialFilterExpression"`
		Collation bson.D `bson:"collation"`
	}
	if err := cursor.All(ctx, &indexes); err != nil {
		return err
	}
	key := slot + ".entries.event_id"
	for _, index := range indexes {
		if len(index.Collation) > 0 {
			simple := false
			for _, field := range index.Collation {
				if field.Key == "locale" && field.Value == "simple" {
					simple = true
				}
			}
			if !simple {
				continue
			}
		}
		if !index.Unique || len(index.Key) != 1 || index.Key[0].Key != key || (index.Key[0].Value != int32(1) && index.Key[0].Value != int64(1)) {
			continue
		}
		if len(index.Partial) != 1 || index.Partial[0].Key != key {
			continue
		}
		value, ok := index.Partial[0].Value.(bson.D)
		if ok && len(value) == 1 && value[0].Key == "$type" && value[0].Value == "string" {
			return nil
		}
	}
	return fmt.Errorf("%w: historical event identity requires the exact unique partial multikey index", ErrUnverifiable)
}

func (s *HistoricalSetSnapshot) Decode(target any) error {
	if s == nil || s.snapshot == nil {
		return ErrUnverifiable
	}
	return s.snapshot.Decode(target)
}

func splitHistoricalSet(raw bson.Raw, slot string) (bson.Raw, *evidence.HistoricalReferenceSetV1, error) {
	if err := raw.Validate(); err != nil {
		return nil, nil, ErrUnverifiable
	}
	if raw.Lookup("_id").Type == 0 {
		return nil, nil, ErrUnverifiable
	}
	var fields bson.D
	if err := bson.Unmarshal(raw, &fields); err != nil {
		return nil, nil, err
	}
	if err := unambiguous(fields); err != nil {
		return nil, nil, err
	}
	stripped, value, err := strip(fields, []string{slot})
	if err != nil {
		return nil, nil, err
	}
	var set *evidence.HistoricalReferenceSetV1
	if raw.Lookup(slot).Type != 0 {
		if raw.Lookup(slot).Type != bson.TypeEmbeddedDocument {
			return nil, nil, ErrUnverifiable
		}
		body, err := bson.Marshal(value)
		if err != nil {
			return nil, nil, err
		}
		set = new(evidence.HistoricalReferenceSetV1)
		if err := bson.Unmarshal(body, set); err != nil {
			return nil, nil, err
		}
		if err := validateHistoricalSetBSON(set); err != nil {
			return nil, nil, err
		}
		// Unknown fields cannot retain an unbounded body in this compact slot.
		canonical, err := bson.Marshal(set)
		if err != nil {
			return nil, nil, err
		}
		var want, actual bson.M
		if err := bson.Unmarshal(canonical, &want); err != nil {
			return nil, nil, err
		}
		if err := bson.Unmarshal(body, &actual); err != nil {
			return nil, nil, err
		}
		if !reflect.DeepEqual(want, actual) {
			return nil, nil, ErrUnverifiable
		}
	}
	base, err := bson.Marshal(stripped)
	return base, set, err
}

// DecodeHistoricalSetDocument validates a narrow projection without allowing
// ignored BSON fields to hide bodies or weaken the persisted typed contract.
func DecodeHistoricalSetDocument(raw bson.Raw, slot string) (*evidence.HistoricalReferenceSetV1, error) {
	if slot != "legacy_submission_evidence" && slot != "historical_generated_evidence" {
		return nil, ErrUnverifiable
	}
	_, set, err := splitHistoricalSet(raw, slot)
	if err == nil && set == nil {
		return nil, ErrUnverifiable
	}
	return set, err
}

func validateHistoricalSetBSON(set *evidence.HistoricalReferenceSetV1) error {
	if set == nil {
		return nil
	}
	if err := set.Validate(); err != nil {
		return err
	}
	encoded, err := bson.Marshal(set)
	if err != nil {
		return err
	}
	if len(encoded) > 512*1024 {
		return fmt.Errorf("%w: historical BSON set exceeds bounded capacity", ErrConflict)
	}
	return nil
}

// Append preserves the complete selected business BSON and every prior entry.
// Even an idempotent append obtains a real transaction write lock; the temporary
// fence is removed in the same borrowed transaction and is never committed alone.
func (s *HistoricalSetSnapshot) Append(ctx context.Context, collection *mongo.Collection, entry evidence.HistoricalReferenceEntryV1) error {
	if err := RequireTransaction(ctx, collection); err != nil {
		return err
	}
	if s == nil || s.snapshot == nil {
		return ErrUnverifiable
	}
	if err := s.snapshot.checkCollection(collection); err != nil {
		return err
	}
	var current bson.Raw
	if err := collection.FindOne(ctx, bson.D{{Key: "_id", Value: s.snapshot.raw.Lookup("_id")}}, options.FindOne().SetCollation(&options.Collation{Locale: "simple"})).Decode(&current); err != nil {
		return err
	}
	slot := s.snapshot.path[0]
	base, set, err := splitHistoricalSet(current, slot)
	if err != nil {
		return err
	}
	if !bytes.Equal(base, s.snapshot.baseline) {
		return ErrConflict
	}
	if s.initial != nil {
		for _, old := range s.initial.Entries {
			var found bool
			if set != nil {
				for _, value := range set.Entries {
					if reflect.DeepEqual(old, value) {
						found = true
						break
					}
				}
			}
			if !found {
				return ErrConflict
			}
		}
	}
	next, err := set.Append(entry)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	if err := validateHistoricalSetBSON(next); err != nil {
		return err
	}
	filter := bson.D{{Key: "_id", Value: current.Lookup("_id")}, {Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$$ROOT", bson.D{{Key: "$literal", Value: current}}}}}}}
	body, err := bson.Marshal(next)
	if err != nil {
		return err
	}
	var fenced bson.D
	if err := bson.Unmarshal(body, &fenced); err != nil {
		return err
	}
	fence := primitive.NewObjectID().Hex()
	fenced = append(fenced, bson.E{Key: "_retirement_cas_fence", Value: fence})
	result, err := collection.UpdateOne(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: slot, Value: fenced}}}}, options.Update().SetCollation(&options.Collation{Locale: "simple"}))
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrConflict
	}
	result, err = collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: current.Lookup("_id")}, {Key: slot + "._retirement_cas_fence", Value: fence}}, bson.D{{Key: "$set", Value: bson.D{{Key: slot, Value: next.Clone()}}}}, options.Update().SetCollation(&options.Collation{Locale: "simple"}))
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return ErrConflict
	}
	return nil
}
