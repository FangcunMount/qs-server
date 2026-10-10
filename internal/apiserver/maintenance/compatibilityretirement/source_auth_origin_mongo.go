package retirement

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"sort"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Canonical Extended JSON is used only for the CLI's metadata protocol. Source
// row hashes and decoders always consume the exact server-returned BSON bytes.
func sourceOriginCanonicalBSON(raw bson.Raw) (any, error) {
	encoded, err := bson.MarshalExtJSON(raw, true, false)
	if err != nil {
		return nil, ErrSourceSchema
	}
	var value any
	if json.Unmarshal(encoded, &value) != nil {
		return nil, ErrSourceSchema
	}
	return value, nil
}
func sourceOriginMongoDefinitionMetadata(metadata mongoCycleMetadata) (SourceBoundary, error) {
	boundary := SourceBoundary{Database: "mongodb", Name: "domain_event_outbox", Kind: "collection", Present: true}
	definition, ok := metadata.definitions[boundary.Name]
	if !ok {
		return boundary, ErrSourceSchema
	}
	if definition.raw.Lookup("type").Type != bson.TypeString || definition.raw.Lookup("type").StringValue() != "collection" {
		return boundary, ErrSourceSchema
	}
	if collation := definition.raw.Lookup("options", "collation", "locale"); collation.Type != 0 && (collation.Type != bson.TypeString || collation.StringValue() != "simple") {
		return boundary, ErrSourceSchema
	}
	collection, err := sourceOriginCanonicalBSON(definition.raw)
	if err != nil {
		return boundary, err
	}
	indices := make([]any, 0, len(definition.indexes))
	idIndex := false
	for _, index := range definition.indexes {
		if index.Lookup("name").Type == bson.TypeString && index.Lookup("name").StringValue() == "_id_" {
			keys, e := exactBSONFields(index.Lookup("key").Document())
			n, integer := mongoExactInteger(keys["_id"])
			if e != nil || len(keys) != 1 || !integer || n != 1 {
				return boundary, ErrSourceSchema
			}
			idIndex = true
			if collation := index.Lookup("collation", "locale"); collation.Type != 0 && (collation.Type != bson.TypeString || collation.StringValue() != "simple") {
				return boundary, ErrSourceSchema
			}
		}
		value, e := sourceOriginCanonicalBSON(index)
		if e != nil {
			return boundary, e
		}
		indices = append(indices, value)
	}
	if !idIndex {
		return boundary, ErrSourceSchema
	}
	sort.Slice(indices, func(i, j int) bool {
		left, _ := sourceOriginDigest(indices[i])
		right, _ := sourceOriginDigest(indices[j])
		return left < right
	})
	boundary.SchemaHash, err = sourceOriginDigest(map[string]any{"collection": collection, "indexes": indices})
	if err != nil {
		return boundary, err
	}
	boundary.IdentityHash = mongoOwnerHashParts("mongodb-object-v1", definition.uuid)
	return boundary, nil
}

func sourceOriginMongoMetadata(ctx context.Context, global *MongoResponsibilitySnapshot, limits SourceOriginLimits) (SourceBoundary, bson.RawValue, error) {
	boundary, err := sourceOriginMongoDefinitionMetadata(global.metadata)
	if err != nil {
		return boundary, bson.RawValue{}, err
	}
	q, cancel := context.WithTimeout(ctx, limits.QueryTimeout)
	defer cancel()
	col := global.db.Collection(boundary.Name)
	// A global type pass prevents a typed range from silently hiding a different
	// BSON _id type. No organization or state predicate is permitted.
	cursor, err := col.Aggregate(q, bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: "$_id"}}}}}}, bson.D{{Key: "$limit", Value: 2}}}, options.Aggregate().SetCollation(&options.Collation{Locale: "simple"}).SetAllowDiskUse(false).SetMaxTime(limits.QueryTimeout))
	if err != nil {
		return boundary, bson.RawValue{}, ErrSourceOrigin
	}
	var types []bson.Raw
	if err = cursor.All(q, &types); err != nil {
		return boundary, bson.RawValue{}, ErrSourceOrigin
	}
	if len(types) > 1 {
		return boundary, bson.RawValue{}, ErrSourceSchema
	}
	var top bson.Raw
	err = col.FindOne(q, bson.D{}, options.FindOne().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: -1}}).SetHint("_id_").SetCollation(&options.Collation{Locale: "simple"}).SetMaxTime(limits.QueryTimeout)).Decode(&top)
	if err == mongo.ErrNoDocuments && len(types) == 0 {
		boundary.Empty = true
		return boundary, bson.RawValue{}, nil
	}
	if err != nil || len(types) != 1 {
		return boundary, bson.RawValue{}, ErrSourceOrigin
	}
	fields, err := exactBSONFields(top)
	if err != nil || len(fields) != 1 {
		return boundary, bson.RawValue{}, ErrSourceSchema
	}
	upper := fields["_id"]
	boundary.PKType = mongoPKType(upper)
	if boundary.PKType == "" || types[0].Lookup("_id").Type != bson.TypeString || types[0].Lookup("_id").StringValue() != boundary.PKType {
		return boundary, bson.RawValue{}, ErrSourceSchema
	}
	token, err := bson.Marshal(bson.D{{Key: "_id", Value: upper}})
	if err != nil {
		return boundary, bson.RawValue{}, ErrSourceBSON
	}
	boundary.UpperToken = base64.StdEncoding.EncodeToString(token)
	return boundary, bson.RawValue{Type: upper.Type, Value: append([]byte(nil), upper.Value...)}, nil
}
func sourceOriginReadMongo(ctx context.Context, global *MongoResponsibilitySnapshot, binding *OriginCopyBinding) (SourceBoundary, SourceCopyReceipt, error) {
	return sourceOriginReadMongoWithInput(ctx, global, binding, binding.alive, nil)
}

func sourceOriginReadMongoWithInput(ctx context.Context, global *MongoResponsibilitySnapshot, binding *OriginCopyBinding, guard func(context.Context) error, freeze sourceOriginInputSink) (SourceBoundary, SourceCopyReceipt, error) {
	if binding == nil || global == nil || guard == nil || guard(ctx) != nil {
		return SourceBoundary{}, SourceCopyReceipt{}, ErrSourceOrigin
	}
	expected := binding.expected[3]
	boundary, upper, err := sourceOriginMongoMetadata(ctx, global, binding.limits)
	if err != nil || boundary != expected.Boundary {
		return boundary, SourceCopyReceipt{}, ErrSourceOrigin
	}
	decoder := &MongoSourceReader{acc: sourceAccumulator{expectation: expected, h: sha256.New()}, upper: upper}
	if !boundary.Empty {
		for page := uint64(0); ; page++ {
			if err = guard(ctx); err != nil {
				return boundary, SourceCopyReceipt{}, err
			}
			if page > binding.limits.MaxRows/uint64(binding.limits.PageRows)+1 {
				return boundary, SourceCopyReceipt{}, ErrSourceBounds
			}
			rangeTerms := bson.D{{Key: "$lte", Value: upper}}
			if decoder.hasLast {
				rangeTerms = append(rangeTerms, bson.E{Key: "$gt", Value: decoder.last})
			}
			q, cancel := context.WithTimeout(ctx, binding.limits.QueryTimeout)
			cursor, e := global.db.Collection(boundary.Name).Find(q, bson.D{{Key: "_id", Value: rangeTerms}}, options.Find().SetHint("_id_").SetCollation(&options.Collation{Locale: "simple"}).SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(binding.limits.PageRows)).SetBatchSize(128).SetMaxTime(binding.limits.QueryTimeout))
			if e != nil {
				cancel()
				return boundary, SourceCopyReceipt{}, ErrSourceOrigin
			}
			n := 0
			var rawPage []bson.Raw
			var pageBytes uint64
			for cursor.Next(q) {
				if e = guard(ctx); e != nil {
					break
				}
				raw := cursor.Current
				if len(raw) > 16<<20 {
					e = ErrSourceBounds
					break
				}
				value, decodeErr := decoder.decodeRaw(raw)
				if decodeErr != nil {
					e = decodeErr
					break
				}
				if e = decoder.acc.add(uint64(len(raw))); e != nil {
					break
				}
				if e = decoder.acc.identities.Observe(value); e != nil {
					break
				}
				sourceFrame(decoder.acc.h, raw, false)
				if e = sourceOriginObserve(binding.copies, value, value.Source.Database, value.Source.Object, value.Source.PrimaryKeySHA256); e != nil {
					break
				}
				if freeze != nil {
					pageBytes += uint64(len(raw))
					if pageBytes > sourceOriginInputPageBytes {
						e = ErrSourceBounds
						break
					}
					rawPage = append(rawPage, append(bson.Raw(nil), raw...))
				}
				n++
			}
			readErr, closeErr := cursor.Err(), cursor.Close(q)
			cancel()
			if e != nil {
				return boundary, SourceCopyReceipt{}, e
			}
			if readErr != nil || closeErr != nil {
				return boundary, SourceCopyReceipt{}, ErrSourceOrigin
			}
			if freeze != nil {
				if e = freeze(ctx, sourceOriginInputFrame{Source: 3, Boundary: boundary, MongoRows: rawPage, EOF: n < binding.limits.PageRows}); e != nil {
					return boundary, SourceCopyReceipt{}, e
				}
			}
			if n < binding.limits.PageRows {
				break
			}
		}
	}
	if boundary.Empty && freeze != nil {
		if err = freeze(ctx, sourceOriginInputFrame{Source: 3, Boundary: boundary, EOF: true}); err != nil {
			return boundary, SourceCopyReceipt{}, err
		}
	}
	if err = decoder.acc.finish(); err != io.EOF {
		return boundary, SourceCopyReceipt{}, err
	}
	return boundary, decoder.Receipt(), nil
}
