package retirement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func mongoCycleCloneValue(v bson.RawValue) bson.RawValue {
	return bson.RawValue{Type: v.Type, Value: append([]byte(nil), v.Value...)}
}
func mongoCycleToken(v bson.RawValue) string {
	return sourceSHA(append([]byte{byte(v.Type)}, v.Value...))
}
func mongoCycleExpectedIDType(name string) bsontype.Type {
	switch name {
	case "rm_outbox":
		return bson.TypeEmbeddedDocument
	case "qs_rm_replay_requests", "evaluation_acceptance_failure_claims", "interpretation_acceptance_failure_claims", "qrcode_acceptance_failure_claims":
		return bson.TypeString
	default:
		return bson.TypeObjectID
	}
}

func mongoCycleAggregateType(t bsontype.Type) string {
	switch t {
	case bson.TypeObjectID:
		return "objectId"
	case bson.TypeEmbeddedDocument:
		return "object"
	case bson.TypeString:
		return "string"
	}
	return "unsupported"
}

// RawValue is passed to the server unchanged, including BSON type and ordered
// document elements. Never use Extended JSON, string ordering or a map cursor.
func mongoCycleRange(lower, upper, last bson.RawValue) bson.D {
	if upper.Type == 0 {
		return bson.D{}
	}
	terms := bson.D{{Key: "$gte", Value: lower}, {Key: "$lte", Value: upper}}
	if last.Type != 0 {
		terms = append(terms, bson.E{Key: "$gt", Value: last})
	}
	return bson.D{{Key: "_id", Value: terms}}
}

func (s *MongoResponsibilitySnapshot) scanCollection(ctx context.Context, name string, fixed *mongoCycleBoundary, classify bool) (mongoCycleBoundary, error) {
	b := mongoCycleBoundary{report: MongoResponsibilityCollectionReport{Collection: name}}
	if err := s.ValidateBorrowedSnapshot(ctx); err != nil {
		return b, err
	}
	definition, present := s.metadata.definitions[name]
	b.report.Present = present
	if !present {
		b.report.RowsSHA256 = mongoOwnerHashParts("mongo-full-bson-rows/v1", name, "absent")
		return b, nil
	}
	b.report.UUID = definition.uuid
	coll := s.db.Collection(name)
	// BSON comparisons are type bracketed. Prove the complete collection has
	// exactly the expected type, rather than silently skipping another type.
	cur, err := coll.Aggregate(ctx, mongo.Pipeline{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: "$_id"}}}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}}, bson.D{{Key: "$limit", Value: 2}}}, options.Aggregate().SetHint("_id_").SetMaxTime(15*time.Second).SetCollation(&options.Collation{Locale: "simple"}))
	if err != nil {
		return b, ErrMongoCycleRead
	}
	var types []struct {
		Type string `bson:"_id"`
		N    int64  `bson:"n"`
	}
	if err = cur.All(ctx, &types); err != nil {
		return b, ErrMongoCycleRead
	}
	if len(types) > 1 {
		return b, ErrMongoCycleSchema
	}
	expected := mongoCycleExpectedIDType(name)
	b.report.IDType = expected.String()
	var total int64
	if len(types) == 1 {
		total = types[0].N
		if total <= 0 || types[0].Type != mongoCycleAggregateType(expected) {
			return b, ErrMongoCycleSchema
		}
	}
	if uint64(total) > s.limits.MaxRows {
		return b, ErrMongoCycleBounds
	}
	if fixed != nil {
		b.lower = mongoCycleCloneValue(fixed.lower)
		b.upper = mongoCycleCloneValue(fixed.upper)
	} else if total > 0 {
		for _, direction := range []int{1, -1} {
			var row bson.Raw
			e := coll.FindOne(ctx, bson.D{}, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: direction}}).SetHint("_id_").SetMaxTime(15*time.Second).SetCollation(&options.Collation{Locale: "simple"})).Decode(&row)
			if e != nil {
				return b, ErrMongoCycleRead
			}
			id := row.Lookup("_id")
			if id.Type != expected {
				return b, ErrMongoCycleSchema
			}
			if direction == 1 {
				b.lower = mongoCycleCloneValue(id)
			} else {
				b.upper = mongoCycleCloneValue(id)
			}
		}
	}
	if b.upper.Type != 0 {
		b.report.LowerTokenSHA256, b.report.UpperTokenSHA256 = mongoCycleToken(b.lower), mongoCycleToken(b.upper)
	}
	h := sha256.New()
	sourceFrame(h, []byte("mongo-full-bson-rows/v1"), false)
	sourceFrame(h, []byte(name), false)
	if total == 0 || fixed != nil && b.upper.Type == 0 {
		b.report.RowsSHA256 = hex.EncodeToString(h.Sum(nil))
		return b, nil
	}
	var last bson.RawValue
	for {
		if err = s.ValidateBorrowedSnapshot(ctx); err != nil {
			return b, err
		}
		s.report.Pages++
		b.report.Pages++
		if s.report.Pages > s.limits.MaxPages {
			return b, ErrMongoCycleBounds
		}
		cur, err = coll.Find(ctx, mongoCycleRange(b.lower, b.upper, last), options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetHint("_id_").SetLimit(int64(s.limits.PageRows)).SetBatchSize(int32(s.limits.PageRows)).SetMaxTime(15*time.Second).SetCollation(&options.Collation{Locale: "simple"}))
		if err != nil {
			return b, ErrMongoCycleRead
		}
		page := 0
		for cur.Next(ctx) {
			raw := cur.Current
			if len(raw) > MaxSourceRowBytes || mongoUniqueBSON(raw, 0) != nil {
				_ = cur.Close(ctx)
				return b, ErrMongoCycleSchema
			}
			id := raw.Lookup("_id")
			if id.Type != expected || last.Type != 0 && bytes.Equal(last.Value, id.Value) {
				_ = cur.Close(ctx)
				return b, ErrMongoCycleSchema
			}
			page++
			b.report.Rows++
			b.report.Bytes += uint64(len(raw))
			s.report.Rows++
			s.report.Bytes += uint64(len(raw))
			if s.report.Rows > s.limits.MaxRows || s.report.Bytes > s.limits.MaxBytes {
				_ = cur.Close(ctx)
				return b, ErrMongoCycleBounds
			}
			sourceFrame(h, raw, false)
			if classify {
				if e := s.classifyRow(name, raw); e != nil {
					_ = cur.Close(ctx)
					return b, e
				}
			}
			last = mongoCycleCloneValue(id)
		}
		cursorErr := cur.Err()
		closeErr := cur.Close(ctx)
		if cursorErr != nil || closeErr != nil {
			return b, ErrMongoCycleRead
		}
		if page == 0 {
			break
		}
	}
	if fixed == nil && (int64(b.report.Rows) != total || !bytes.Equal(last.Value, b.upper.Value)) {
		return b, ErrMongoCycleConflict
	}
	b.report.RowsSHA256 = hex.EncodeToString(h.Sum(nil))
	return b, nil
}

func (s *MongoResponsibilitySnapshot) hasOutsideRows(ctx context.Context, name string, b mongoCycleBoundary) (bool, error) {
	if !b.report.Present {
		return false, nil
	}
	filter := bson.D{}
	if b.upper.Type != 0 {
		filter = bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: b.lower}}}}, bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: b.upper}}}}}}}
	}
	var row bson.Raw
	err := s.db.Collection(name).FindOne(ctx, filter, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetHint("_id_").SetMaxTime(15*time.Second).SetCollation(&options.Collation{Locale: "simple"})).Decode(&row)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return false, nil
	}
	if err != nil {
		return false, ErrMongoCycleRead
	}
	return true, nil
}

func (s *MongoResponsibilitySnapshot) snapshotHash() string {
	parts := []string{"mongo-global-responsibility-snapshot/v1", s.metadata.hash}
	for _, c := range s.report.Collections {
		parts = append(parts, c.Collection, c.UUID, c.LowerTokenSHA256, c.UpperTokenSHA256, c.RowsSHA256)
	}
	return mongoOwnerHashParts(parts...)
}
